package service

import (
	"bytes"
	"fmt"
	"text/template"
)

const (
	// DefaultNetfilterHookPath — путь к NDM netfilter.d hook на Keenetic.
	// NDM вызывает все исполняемые файлы из этой директории после rebuild
	// iptables/ip6tables/ipset (привязка устройства к policy, save startup-config,
	// reconnect WAN). Без hook сторонние chain'ы (signcraze_policy) теряются.
	DefaultNetfilterHookPath = "/opt/etc/ndm/netfilter.d/50-sign-craze"

	// DefaultCorePIDPath — fallback путь к PID-файлу прокси-ядра при
	// HookParams.PIDPath не задан. Совпадает с singbox.DefaultPIDFile.
	// Используется в тестах и для legacy install-флоу до миграции на multi-core.
	DefaultCorePIDPath = "/opt/var/run/sign-craze-singbox.pid"
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
#
# Сериализация через flock + trailing debounce: при пакете NDM-событий
# (например 10 устройств в policy подряд) первый процесс захватывает
# lock и делает reapply, остальные ставят pending-маркер и моментально
# выходят. После reapply lock-holder спит 2с, проверяет pending — если
# есть, удаляет и делает второй финальный reapply. Это снимает CPU-шторм
# на slow MIPS и гарантирует что финальное состояние всё равно
# зареапплаится.
SC={{ .BinPath }}
LOCK=/opt/var/run/sign-craze-hook.lock
PENDING=/opt/var/run/sign-craze-hook.pending
[ -x "$SC" ] || exit 0
[ -f {{ .PIDPath }} ] || exit 0
case "${type:-iptables}/${table:-mangle}" in
    iptables/mangle|ip6tables/mangle|ipset/*) ;;
    *) exit 0 ;;
esac
# Fallback если flock недоступен (минимальный Entware без util-linux).
if ! command -v flock >/dev/null 2>&1; then
    "$SC" --reapply >/dev/null 2>&1 || true
    exit 0
fi
exec 9>"$LOCK"
if ! flock -x -n 9; then
    touch "$PENDING"
    exit 0
fi
"$SC" --reapply >/dev/null 2>&1 || true
sleep 2
if [ -f "$PENDING" ]; then
    rm -f "$PENDING"
    "$SC" --reapply >/dev/null 2>&1 || true
fi
`))

// HookParams задаёт параметры для генерации netfilter.d hook.
type HookParams struct {
	BinPath string // путь к бинарю sign-craze; по умолчанию DefaultSignCrazeBin
	// PIDPath — путь к PID-файлу активного прокси-ядра (sing-box/xray/mihomo).
	// Hook делает раннее exit 0 если файл отсутствует — чтобы reapply не выполнялся
	// при остановленном сервисе. По умолчанию DefaultCorePIDPath (sing-box-совместимо).
	// При смене core через --core <name> hook регенерируется с новым PIDPath.
	PIDPath string
}

// WriteHook генерирует netfilter.d hook и атомарно записывает его в hookPath.
// Идемпотентно: если файл существует и содержимое совпадает (SHA256) — пропускает.
// Рендеринг делегирован RenderHook, запись — общему хелперу writeIfChanged
// (writeutil.go), который использует также WriteShim.
func WriteHook(hookPath string, p HookParams) error {
	content, err := RenderHook(p)
	if err != nil {
		return err
	}

	_, err = writeIfChanged(hookPath, content, 0o755, "netfilter.d hook")
	return err
}

// RenderHook возвращает содержимое hook без записи на диск (для тестов).
func RenderHook(p HookParams) ([]byte, error) {
	applyHookDefaults(&p)
	var buf bytes.Buffer
	if err := hookTemplate.Execute(&buf, p); err != nil {
		return nil, fmt.Errorf("netfilter hook: рендеринг: %w", err)
	}
	return buf.Bytes(), nil
}

func applyHookDefaults(p *HookParams) {
	if p.BinPath == "" {
		p.BinPath = DefaultSignCrazeBin
	}
	if p.PIDPath == "" {
		p.PIDPath = DefaultCorePIDPath
	}
}
