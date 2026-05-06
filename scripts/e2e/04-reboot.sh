#!/bin/sh
# 04-reboot.sh — перезагружает роутер и проверяет автостарт через init.d shim.
# Запускать на хосте разработчика.
set -eu

KEENETIC_HOST="${KEENETIC_HOST:?Укажите KEENETIC_HOST}"
KEENETIC_USER="${KEENETIC_USER:-root}"
KEENETIC_KEY="${KEENETIC_KEY:-${HOME}/.ssh/keenetic}"
REBOOT_TIMEOUT=180  # секунд ожидания восстановления SSH
POLL_INTERVAL=5     # секунд между попытками
STEP="04-reboot"

log()  { printf '[%s] %s\n' "${STEP}" "$*"; }
fail() { log "FAIL:${STEP}: $*"; exit 1; }
pass() { log "PASS:${STEP}"; }

ssh_cmd() {
    # shellcheck disable=SC2029
    ssh -i "${KEENETIC_KEY}" -o StrictHostKeyChecking=no \
        -o ConnectTimeout=5 \
        "${KEENETIC_USER}@${KEENETIC_HOST}" "$@"
}

# ---------- инициируем перезагрузку ----------
log "Отправляем команду reboot на роутер..."
# Запускаем reboot в фоне через & чтобы SSH-сессия не зависла
ssh_cmd "reboot &" 2>/dev/null || true

# ---------- ждём завершения перезагрузки ----------
log "Ожидаем восстановления SSH (timeout ${REBOOT_TIMEOUT}s)..."

elapsed=0
available=0

# Сначала дадим роутеру время начать выключение (минимум 5 секунд)
sleep 5

while [ "${elapsed}" -lt "${REBOOT_TIMEOUT}" ]; do
    if ssh_cmd true 2>/dev/null; then
        available=1
        break
    fi
    sleep "${POLL_INTERVAL}"
    elapsed=$((elapsed + POLL_INTERVAL))
    log "  ожидаем... ${elapsed}/${REBOOT_TIMEOUT}s"
done

[ "${available}" -eq 1 ] \
    || fail "роутер не ответил по SSH за ${REBOOT_TIMEOUT}s"

log "SSH доступен, роутер перезагрузился."

# ---------- проверяем --status ----------
log "Проверяем --status после перезагрузки..."
status_out=$(ssh_cmd "/opt/sbin/sign-craze --status" 2>&1) || true
echo "${status_out}" | grep -qi "running" \
    || fail "--status не RUNNING после перезагрузки: ${status_out}"

# ---------- проверяем uptime sign-craze ----------
# Процесс должен запуститься через init.d shim при старте, uptime < 60s
log "Проверяем, что sign-craze запущен недавно (uptime <60s)..."
uptime_out=$(ssh_cmd "
    pid=\$(cat /opt/var/run/sign-craze.pid 2>/dev/null || echo '')
    if [ -n \"\${pid}\" ]; then
        # /proc/<pid>/stat: поле 22 — время старта в jiffies с boot
        proc_start=\$(awk '{print \$22}' /proc/\${pid}/stat 2>/dev/null || echo 0)
        uptime_s=\$(awk '{print int(\$1)}' /proc/uptime 2>/dev/null || echo 9999)
        hz=100
        proc_uptime=\$((uptime_s - proc_start / hz))
        echo \"\${proc_uptime}\"
    else
        echo 'no_pid'
    fi
" 2>&1) || true

if [ "${uptime_out}" = "no_pid" ] || [ -z "${uptime_out}" ]; then
    log "Предупреждение: PID-файл sign-craze не найден, пропускаем проверку uptime."
elif [ "${uptime_out}" -gt 60 ] 2>/dev/null; then
    fail "sign-craze работает слишком долго до перезагрузки? uptime=${uptime_out}s"
else
    log "sign-craze uptime: ${uptime_out}s (в норме)"
fi

pass
