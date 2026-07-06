package service

import (
	"bytes"
	"fmt"
	"text/template"
)

const (
	// DefaultShimPath — путь к init.d shim на роутере.
	// S99 (а не S05) выбран намеренно: rc.unslung исполняет init.d-скрипты
	// в лексикографическом порядке, и любой `set -eu`-fail в shim прерывает
	// последовательность. На S05 это блокировало запуск S51dropbear → SSH 222
	// становился недоступен после reboot (инцидент 2026-05-07). На S99 shim
	// запускается ПОСЛЕ S51dropbear → даже падение sign-craze не убивает SSH.
	DefaultShimPath = "/opt/etc/init.d/S99signcraze"
	// DefaultBinPath — путь к бинарю sign-craze на роутере.
	DefaultSignCrazeBin = "/opt/sbin/sign-craze"
	// DefaultWatchdogPIDPath — PID-файл standalone firewall watchdog daemon.
	// Запись делает init.d shim (start_watchdog), читают --stop / --uninstall
	// чтобы корректно остановить демон. Без этого watchdog продолжает крутить
	// reconcileFirewall после удаления sing-box и пересоздаёт state.json через
	// ensureKeeneticPolicy → saveState (см. issue v0.4.2 install/uninstall).
	DefaultWatchdogPIDPath = "/opt/var/run/sign-craze-watchdog.pid"
)

// shimTemplate — шаблон init.d shim (~30 строк, генерируется sign-craze).
// set -eu + логирование stderr в boot.log: иначе init молча игнорирует
// падение --service-start, и юзер думает что сервис стартовал (safety-fixes #5).
// status не пишет в boot.log — он опрашивается часто и полезен в stderr.
var shimTemplate = template.Must(template.New("shim").Parse(`#!/bin/sh
# Сгенерировано sign-craze. Не редактировать вручную.
set -eu
SC={{ .BinPath }}
LOG=/opt/var/log/sign-craze/boot.log
WATCHDOG_PID=/opt/var/run/sign-craze-watchdog.pid
mkdir -p "$(dirname "$LOG")" "$(dirname "$WATCHDOG_PID")" 2>/dev/null || true

run() {
    if ! "$SC" "$1" 2>>"$LOG"; then
        echo "sign-craze $1 завершился с ошибкой (см. $LOG)" >&2
        exit 1
    fi
}

# start_watchdog запускает standalone firewall watchdog в фоне.
# Идемпотентно: если PID-файл указывает на живой процесс — пропуск.
# Watchdog нужен чтобы пережить ndm rebuild FORWARD chain через несколько
# часов после boot. Без него правила теряются и трафик клиентов дропается.
start_watchdog() {
    if [ -f "$WATCHDOG_PID" ]; then
        OLD_PID=$(cat "$WATCHDOG_PID" 2>/dev/null || echo "")
        if [ -n "$OLD_PID" ] && [ -d "/proc/$OLD_PID" ]; then
            return 0
        fi
    fi
    nohup "$SC" --service-watchdog >>"$LOG" 2>&1 &
    echo $! > "$WATCHDOG_PID"
}

stop_watchdog() {
    if [ -f "$WATCHDOG_PID" ]; then
        WD_PID=$(cat "$WATCHDOG_PID" 2>/dev/null || echo "")
        if [ -n "$WD_PID" ] && [ -d "/proc/$WD_PID" ]; then
            kill -TERM "$WD_PID" 2>/dev/null || true
        fi
        rm -f "$WATCHDOG_PID"
    fi
}

case "${1:-}" in
    start)   run --service-start; start_watchdog ;;
    stop)    stop_watchdog; run --stop ;;
    restart) stop_watchdog; run --restart; start_watchdog ;;
    status)  exec "$SC" --status ;;
    *)       echo "Использование: $0 {start|stop|restart|status}" >&2; exit 1 ;;
esac
`))

// ShimParams задаёт параметры для генерации init.d shim.
type ShimParams struct {
	BinPath string // путь к бинарю sign-craze; по умолчанию DefaultSignCrazeBin
}

// WriteShim генерирует init.d shim и атомарно записывает его в shimPath.
// Если файл уже существует и содержимое совпадает (по SHA256) — запись пропускается.
// Рендеринг делегирован RenderShim (единственный источник шаблонной логики,
// не дублируем здесь), запись — общему хелперу writeIfChanged (writeutil.go),
// который использует также WriteHook.
func WriteShim(shimPath string, p ShimParams) error {
	content, err := RenderShim(p)
	if err != nil {
		return err
	}

	_, err = writeIfChanged(shimPath, content, 0o755, "init.d shim")
	return err
}

// RenderShim возвращает содержимое shim без записи на диск (используется в тестах).
func RenderShim(p ShimParams) ([]byte, error) {
	if p.BinPath == "" {
		p.BinPath = DefaultSignCrazeBin
	}
	var buf bytes.Buffer
	if err := shimTemplate.Execute(&buf, p); err != nil {
		return nil, fmt.Errorf("service shim: рендеринг: %w", err)
	}
	return buf.Bytes(), nil
}
