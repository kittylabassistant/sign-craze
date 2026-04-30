package service

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"text/template"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

const (
	// DefaultNetfilterHookPath — путь к NDM netfilter.d hook на Keenetic.
	// NDM вызывает все исполняемые файлы из этой директории после rebuild
	// iptables/ip6tables/ipset (привязка устройства к policy, save startup-config,
	// reconnect WAN). Без hook сторонние chain'ы (signcraze_policy) теряются.
	DefaultNetfilterHookPath = "/opt/etc/ndm/netfilter.d/50-sign-craze"
)

// hookTemplate — шаблон NDM netfilter.d hook (POSIX sh).
//
// NDM передаёт env-переменные:
//
//	type  — iptables | ip6tables | ipset
//	table — mangle | nat | filter | raw (для iptables/ip6tables)
//
// Реагируем только на mangle (наши правила там) и ipset (для full mode).
// Если сервис не запущен — exit 0 без действий.
var hookTemplate = template.Must(template.New("hook").Parse(`#!/bin/sh
# Сгенерировано sign-craze. Не редактировать вручную.
# NDM вызывает hook после rebuild iptables; реапплаим mangle-правила.
# NDM env: type=iptables|ip6tables|ipset, table=mangle|nat|filter
SC={{ .BinPath }}
[ -x "$SC" ] || exit 0
[ -f /opt/var/run/sign-craze-singbox.pid ] || exit 0
case "${type:-iptables}/${table:-mangle}" in
    iptables/mangle|ip6tables/mangle|ipset/*) ;;
    *) exit 0 ;;
esac
"$SC" --reapply >/dev/null 2>&1 || true
`))

// HookParams задаёт параметры для генерации netfilter.d hook.
type HookParams struct {
	BinPath string // путь к бинарю sign-craze; по умолчанию DefaultSignCrazeBin
}

// WriteHook генерирует netfilter.d hook и атомарно записывает его в hookPath.
// Идемпотентно: если файл существует и содержимое совпадает (SHA256) — пропускает.
func WriteHook(hookPath string, p HookParams) error {
	if p.BinPath == "" {
		p.BinPath = DefaultSignCrazeBin
	}

	var buf bytes.Buffer
	if err := hookTemplate.Execute(&buf, p); err != nil {
		return fmt.Errorf("netfilter hook: рендеринг шаблона: %w", err)
	}
	content := buf.Bytes()

	if existing, err := os.ReadFile(hookPath); err == nil {
		if sha256.Sum256(existing) == sha256.Sum256(content) {
			log.L().Debug("netfilter hook актуален, запись пропущена", "path", hookPath)
			return nil
		}
	}

	if err := atomicfs.WriteFileAtomic(hookPath, content, 0o755); err != nil {
		return fmt.Errorf("netfilter hook: запись файла: %w", err)
	}

	log.L().Info("netfilter.d hook записан", "path", hookPath)
	return nil
}

// RenderHook возвращает содержимое hook без записи на диск (для тестов).
func RenderHook(p HookParams) ([]byte, error) {
	if p.BinPath == "" {
		p.BinPath = DefaultSignCrazeBin
	}
	var buf bytes.Buffer
	if err := hookTemplate.Execute(&buf, p); err != nil {
		return nil, fmt.Errorf("netfilter hook: рендеринг: %w", err)
	}
	return buf.Bytes(), nil
}
