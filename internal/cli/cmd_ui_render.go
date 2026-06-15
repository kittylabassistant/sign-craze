package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// freshStateOr возвращает свежий state.Load или fallback. Используется в UI
// closures для re-resolve активного state без перезапуска daemon'а.
func freshStateOr(fallback *state.State) *state.State {
	if fresh, err := state.Load(state.DefaultPath); err == nil && fresh != nil {
		return fresh
	}
	return fallback
}

// uiRenderParams строит CoreRenderParams для UI preview/render: state.Mode +
// state.Outbounds + клиентский routing config (cfg) + InboundMode + DefaultOutboundTag.
// Используется Renderer callback в RoutingUIDeps.
func uiRenderParams(st *state.State, cfg *types.RoutingConfig) types.CoreRenderParams {
	p := types.CoreRenderParams{
		RoutingConfig: cfg,
	}
	if st != nil {
		p.Mode = st.Mode
		p.Outbounds = st.Outbounds
		p.InboundMode = st.Inbound
		if len(st.Outbounds) > 0 {
			p.DefaultOutboundTag = st.Outbounds[0].Tag
		}
	}
	if p.DefaultOutboundTag == "" && cfg != nil && len(cfg.Outbounds) > 0 {
		p.DefaultOutboundTag = cfg.Outbounds[0].Tag
	}
	return p
}

// validateRoutingConfig валидирует RoutingConfig через активное ядро.
//
// Шаги:
//  1. Рендер через c.RenderConfig(uiRenderParams) — выявит protocol-уровневые
//     ошибки (несовместимый outbound и т.д.).
//  2. Запись в tmp-файл с правильным расширением (json/yaml).
//  3. c.CheckConfig(tmp) — запускает sing-box check / xray test / mihomo -t,
//     ловит синтаксис и semantical ошибки на native уровне.
//  4. Сбор warnings: TUN inbound при xray/mihomo, RuleSet с несовместимым
//     форматом URL и т.д.
//  5. Cleanup tmp-файла.
//
// Возвращает (warnings, fatal-error). Warnings не делают config invalid —
// они подсвечивают compatibility-проблемы для UI.
func validateRoutingConfig(ctx context.Context, runner exectx.Runner, cfg *types.RoutingConfig, st *state.State) ([]string, error) {
	c := mustActiveCore()
	want, err := c.RenderConfig(uiRenderParams(st, cfg))
	if err != nil {
		return nil, fmt.Errorf("render (%s): %w", c.Name(), err)
	}

	ext := "json"
	if c.ConfigFormat() == core.ConfigYAML {
		ext = "yaml"
	}
	tmp, err := os.CreateTemp("", "sign-craze-validate-*."+ext)
	if err != nil {
		return nil, fmt.Errorf("создание tmp-файла: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.L().Debug("validate: cleanup tmp", "path", tmpPath, "err", removeErr)
		}
	}()
	tmp.Close() // содержимое запишет atomicfs.WriteFileAtomic

	if writeErr := atomicfs.WriteFileAtomic(tmpPath, want, 0o640); writeErr != nil {
		return nil, fmt.Errorf("запись tmp-конфига: %w", writeErr)
	}

	// CheckConfig для core может быть невозможен (бинарь не установлен на
	// dev-машине). Surface как warning, не fatal — UI всё равно покажет ok.
	warnings := collectCompatWarnings(c, cfg)
	if _, statErr := os.Stat(c.BinaryPath()); statErr == nil {
		if checkErr := c.CheckConfig(ctx, runner, tmpPath); checkErr != nil {
			return warnings, fmt.Errorf("%s check: %w", c.Name(), checkErr)
		}
	} else {
		warnings = append(warnings,
			fmt.Sprintf("бинарь %s не установлен в %s, native validation пропущена", c.Name(), c.BinaryPath()))
	}
	return warnings, nil
}

// collectCompatWarnings собирает compatibility warnings без запуска CheckConfig.
// Подсвечивает несовместимости между сохранённым routing.json и активным ядром:
//   - TUN inbound + core==xray/mihomo (TUN не поддерживается, dropped)
//   - RuleSet с .srs URL + core==mihomo (нужен .mrs)
//   - RuleSet без префикса geosite-/geoip- + core==xray (нельзя translate в matcher)
func collectCompatWarnings(c core.Core, cfg *types.RoutingConfig) []string {
	if cfg == nil {
		return nil
	}
	var warnings []string

	if c.Name() != "sing-box" {
		for _, ib := range cfg.Inbounds {
			if ib.Type == "tun" {
				warnings = append(warnings,
					fmt.Sprintf("inbound %q (type=tun): %s не поддерживает TUN, inbound будет проигнорирован при рендере", ib.Tag, c.Name()))
			}
		}
	}

	switch c.GeoFormat() {
	case core.GeoMRS:
		for _, rs := range cfg.RuleSets {
			if rs.URL != "" && !strings.HasSuffix(rs.URL, ".mrs") {
				warnings = append(warnings,
					fmt.Sprintf("rule_set %q: URL %q, mihomo требует .mrs — применить preset заново для активного ядра", rs.Tag, rs.URL))
			}
		}
	case core.GeoDAT:
		// xray использует translation RuleSet→geosite:/geoip: matcher.
		// Без префикса translation невозможна.
		for _, rs := range cfg.RuleSets {
			if !strings.HasPrefix(rs.Tag, "geosite-") && !strings.HasPrefix(rs.Tag, "geoip-") {
				warnings = append(warnings,
					fmt.Sprintf("rule_set %q: для xray translation работает только с префиксом geosite-/geoip-, matcher будет пропущен", rs.Tag))
			}
		}
	}

	return warnings
}
