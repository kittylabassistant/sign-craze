package singbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"

	_ "embed"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

//go:embed templates/tun.json.tmpl
var tunTmpl string

// configFuncMap — функции, доступные в template/tun.json.tmpl. Pure-функции,
// не захватывают состояние Render, поэтому шаблон можно скомпилировать один
// раз на уровне пакета (см. configTmpl ниже).
var configFuncMap = template.FuncMap{
	"jsonMarshal": func(v any) (string, error) {
		// Для types.Outbound используем renderOutboundJSON чтобы получить
		// sing-box-специфичный flatten (canonical или legacy Settings).
		if ob, ok := v.(types.Outbound); ok {
			m, rErr := renderOutboundJSON(ob)
			if rErr != nil {
				return "", rErr
			}
			b, mErr := json.Marshal(m)
			if mErr != nil {
				return "", mErr
			}
			return string(b), nil
		}
		b, mErr := json.Marshal(v)
		if mErr != nil {
			return "", mErr
		}
		return string(b), nil
	},
}

// configTmpl — предкомпилированный шаблон tun.json. Парсинг шаблона
// (text/template) на MIPS softfloat занимает десятки ms; делаем это однократно
// в init-time, чтобы каждый Render() и watchdog Reconcile не платил повторно.
// Init-time panic при битом шаблоне — приемлемо: шаблон embedded и его валидность
// проверяется тестами.
var configTmpl = template.Must(template.New("config").Funcs(configFuncMap).Parse(tunTmpl))

// Defaults для TUN-inbound.
//
// MTU 1280 (минимум IPv6) выбран чтобы избежать фрагментации на upstream-пути
// VLESS+Reality+gRPC: outer-encapsulation добавляет ~60-100 байт TLS+gRPC+H2
// overhead, а path MTU до homelabcloud.ru может быть меньше 1500 из-за
// PPPoE/туннелей у провайдера. На mipsle softfloat + gvisor TCP stack крупные
// сегменты (>1280) с PSH ломают handshake (server retransmits 544-byte
// Finished не доставляются клиенту). 1280 даёт запас и для PMTUD-чёрных дыр.
const (
	DefaultTUNInterfaceName = "signbox-tun"
	DefaultTUNMTU           = 1280
)

// Defaults для TPROXY-inbound.
//
// TPROXY сохраняет оригинальный src/dst LAN-клиента — sing-box видит
// 172.16.0.97 (RPi4) вместо 172.19.0.1 (TUN gateway). Порт 7895
// совпадает с портом в Config.Port firewall-слоя.
const (
	DefaultTProxyPort uint16 = 7895
	DefaultTProxyTag         = "tproxy-in"
)

// DefaultTUNAddresses — IPv4 /30 + IPv6 /126 для transparent gateway внутри TUN.
// 172.19.0.0/30 и fdfe:dcba:9876::/126 не пересекаются с типичными Keenetic LAN-подсетями.
var DefaultTUNAddresses = []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"}

// validLogLevels — разрешённые значения LogLevel sing-box. Whitelisted, чтобы
// state.json с инжекцией ("info\n--evil") не утёк в config.json.
var validLogLevels = map[string]bool{
	"trace": true,
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
	"fatal": true,
	"panic": true,
}

// ConfigParams задаёт параметры для генерации конфига sing-box.
type ConfigParams struct {
	Mode     types.Mode
	LogLevel string // "info", "debug", "warn", "error"

	// TUN inbound: sing-box создаёт TUN-интерфейс при старте. Маршрутизация
	// (ip rule fwmark → table → default dev <TUN>) делается отдельно
	// firewall-слоем; sing-box auto_route отключён.
	TUNInterfaceName string   // default "signbox-tun"
	TUNAddresses     []string // default ["172.19.0.1/30", "fdfe:dcba:9876::1/126"]
	TUNMTU           int      // default 1280

	Outbounds          []types.Outbound
	DefaultOutboundTag string // тег первого outbound, используемый как final

	// RoutingConfig — пользовательская конфигурация маршрутизации.
	// Если задан — outbounds, rules и rule_sets берутся из него.
	RoutingConfig *types.RoutingConfig

	// InboundMode — режим входящего соединения: "tun" или "tproxy".
	// Пустая строка трактуется как "tun" (backward compat).
	// При "tproxy" и пустом RoutingConfig.Inbounds Render() добавляет
	// TPROXY inbound с портом TProxyPort. При "tun" — поведение без изменений.
	InboundMode string
	// TProxyPort — порт TPROXY-inbound sing-box. 0 → DefaultTProxyPort (7895).
	TProxyPort uint16
	// TProxyTag — тег TPROXY-inbound. Пустой → DefaultTProxyTag ("tproxy-in").
	TProxyTag string
	// TProxyKernelOK — реальный TPROXY доступен (xt_TPROXY загружен).
	// При true — Render() выставляет Type: "tproxy" с listen "::" (TCP+UDP).
	// При false — fallback "redirect" с listen "0.0.0.0" (TCP-only).
	// Заполняется applier'ом при probe xt_TPROXY в applyPolicyMode.
	TProxyKernelOK bool
}

// DefaultConfigParams возвращает ConfigParams с разумными значениями по умолчанию.
func DefaultConfigParams() ConfigParams {
	return ConfigParams{
		Mode:             types.ModePolicy,
		LogLevel:         "info",
		TUNInterfaceName: DefaultTUNInterfaceName,
		TUNAddresses:     append([]string(nil), DefaultTUNAddresses...),
		TUNMTU:           DefaultTUNMTU,
	}
}

// Render генерирует содержимое config.json из шаблона и параметров.
// Возвращает валидный JSON или ошибку.
func Render(p ConfigParams) ([]byte, error) {
	// defaults TUN
	if p.TUNInterfaceName == "" {
		p.TUNInterfaceName = DefaultTUNInterfaceName
	}
	if len(p.TUNAddresses) == 0 {
		p.TUNAddresses = append([]string(nil), DefaultTUNAddresses...)
	}
	if p.TUNMTU == 0 {
		p.TUNMTU = DefaultTUNMTU
	}
	if p.LogLevel == "" {
		p.LogLevel = "info"
	}
	if !validLogLevels[p.LogLevel] {
		return nil, fmt.Errorf("singbox config: некорректный LogLevel %q (допустимо: trace/debug/info/warn/error/fatal/panic)", p.LogLevel)
	}

	// TPROXY-режим: если InboundMode == "tproxy" и RoutingConfig не задаёт
	// inbounds явно — синтезируем inbound через RoutingConfig.Inbounds.
	// Это позволяет шаблону tun.json.tmpl рендерить кастомный inbound вместо
	// default TUN-блока (ветка {{if .Inbounds}}).
	//
	// При TProxyKernelOK=true — Type: "tproxy", listen: "::" (TCP+UDP, ядро xt_TPROXY загружено).
	// При TProxyKernelOK=false — Type: "redirect", listen: "0.0.0.0" (TCP-only, fallback).
	if p.InboundMode == "tproxy" {
		port := p.TProxyPort
		if port == 0 {
			port = DefaultTProxyPort
		}
		tag := p.TProxyTag
		if tag == "" {
			tag = DefaultTProxyTag
		}
		hasExplicitInbounds := p.RoutingConfig != nil && len(p.RoutingConfig.Inbounds) > 0
		if !hasExplicitInbounds {
			var tproxyInbound types.Inbound
			if p.TProxyKernelOK {
				// Реальный TPROXY: xt_TPROXY загружен, sing-box получает оригинальный
				// src/dst LAN-клиента. Listen "::" принимает TCP и UDP на всех интерфейсах.
				tproxyInbound = types.Inbound{
					Tag:  tag,
					Type: "tproxy",
					Settings: map[string]any{
						"listen":      "::",
						"listen_port": port,
					},
				}
			} else {
				// Fallback REDIRECT: xt_TPROXY отсутствует в kernel Keenetic 4.9.
				// REDIRECT меняет dst на адрес интерфейса (br0 → 172.16.0.1).
				// Sing-box listen на 0.0.0.0 чтобы принимать на любом локальном IP.
				// SO_ORIGINAL_DST извлекает original destination; src клиента сохраняется.
				// Работает только для TCP; UDP не проксируется.
				//
				// listen 127.0.0.1 не подходит: packet с не-loopback src
				// (172.16.0.97) на dst=127.0.0.1 считается martian и
				// дропается kernel'ом до доставки в socket.
				tproxyInbound = types.Inbound{
					Tag:  tag,
					Type: "redirect",
					Settings: map[string]any{
						"listen":      "0.0.0.0",
						"listen_port": port,
					},
				}
			}
			if p.RoutingConfig == nil {
				p.RoutingConfig = &types.RoutingConfig{
					Version:   1,
					Inbounds:  []types.Inbound{tproxyInbound},
					Outbounds: p.Outbounds,
					Final:     p.DefaultOutboundTag,
				}
				if p.RoutingConfig.Final == "" && len(p.Outbounds) > 0 {
					p.RoutingConfig.Final = p.Outbounds[0].Tag
				}
			} else {
				p.RoutingConfig.Inbounds = []types.Inbound{tproxyInbound}
			}
		}
	}

	if p.RoutingConfig != nil {
		// новый путь: RoutingConfig обязан пройти Validate
		if err := p.RoutingConfig.Validate(); err != nil {
			return nil, fmt.Errorf("singbox config: RoutingConfig невалиден: %w", err)
		}
	} else {
		// legacy: валидация Outbounds + DefaultOutboundTag fallback
		if p.DefaultOutboundTag == "" && len(p.Outbounds) > 0 {
			p.DefaultOutboundTag = p.Outbounds[0].Tag
		}
		for _, o := range p.Outbounds {
			if err := o.Validate(); err != nil {
				return nil, fmt.Errorf("singbox config: %w", err)
			}
		}
	}

	model, err := buildEffectiveModel(p)
	if err != nil {
		return nil, fmt.Errorf("singbox config: построение модели: %w", err)
	}

	var buf bytes.Buffer
	if err := configTmpl.Execute(&buf, model); err != nil {
		return nil, fmt.Errorf("singbox config: рендеринг шаблона: %w", err)
	}

	out := buf.Bytes()

	// TODO(perf): json.Valid — защитная проверка на случай ошибки в шаблоне.
	// WriteConfig дополнительно валидирует через `sing-box check -c`.
	// На MIPS softfloat json.Valid для типичного конфига (~4KB) занимает <1ms,
	// поэтому оставляем как early-catch без build-tag.
	if !json.Valid(out) {
		return nil, fmt.Errorf("singbox config: сгенерированный JSON невалиден")
	}

	return out, nil
}

// WriteConfig рендерит конфиг и атомарно записывает его в path.
func WriteConfig(ctx context.Context, runner exectx.Runner, p ConfigParams, binPath, configPath string) error {
	data, err := Render(p)
	if err != nil {
		return err
	}

	if err := atomicfs.WriteFileAtomic(configPath, data, 0o640); err != nil {
		return fmt.Errorf("singbox config: запись файла: %w", err)
	}

	// валидация через sing-box check -c
	if err := checkConfig(ctx, runner, binPath, configPath); err != nil {
		return fmt.Errorf("singbox config: %w", err)
	}

	log.L().Info("конфиг sing-box записан", "path", configPath)
	return nil
}
