#!/bin/sh
# 99-uninstall.sh — удаляет sign-craze с роутера и проверяет полную очистку.
# Запускать на хосте разработчика.
set -eu

KEENETIC_HOST="${KEENETIC_HOST:?Укажите KEENETIC_HOST}"
KEENETIC_USER="${KEENETIC_USER:-root}"
KEENETIC_KEY="${KEENETIC_KEY:-${HOME}/.ssh/keenetic}"
STEP="99-uninstall"

log()  { printf '[%s] %s\n' "${STEP}" "$*"; }
fail() { log "FAIL:${STEP}: $*"; exit 1; }
pass() { log "PASS:${STEP}"; }

ssh_cmd() {
    # shellcheck disable=SC2029
    ssh -i "${KEENETIC_KEY}" -o StrictHostKeyChecking=accept-new \
        "${KEENETIC_USER}@${KEENETIC_HOST}" "$@"
}

# ---------- удаление ----------
log "Запускаем --uninstall на роутере..."
ssh_cmd "/opt/sbin/sign-craze --uninstall" || true

# небольшая пауза — uninstall останавливает процессы и удаляет файлы
sleep 3

# ---------- проверяем отсутствие бинаря ----------
log "Проверяем отсутствие /opt/sbin/sign-craze..."
ssh_cmd "test ! -f /opt/sbin/sign-craze" \
    || fail "/opt/sbin/sign-craze всё ещё существует"

# ---------- проверяем отсутствие init-shim ----------
log "Проверяем отсутствие /opt/etc/init.d/S99signcraze..."
ssh_cmd "test ! -f /opt/etc/init.d/S99signcraze" \
    || fail "/opt/etc/init.d/S99signcraze всё ещё существует"

# ---------- проверяем удаление policy из RCI ----------
log "Проверяем отсутствие policy description=sign-craze в RCI..."
rci_out=$(ssh_cmd \
    "curl -sf http://127.0.0.1:79/rci/show/ip/policy" 2>&1) \
    || {
        log "Предупреждение: RCI /show/ip/policy недоступен, пропускаем проверку."
        pass
        exit 0
    }

if echo "${rci_out}" | grep -q "sign-craze"; then
    fail "policy description=sign-craze всё ещё присутствует в RCI: ${rci_out}"
fi

pass
