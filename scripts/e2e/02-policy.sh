#!/bin/sh
# 02-policy.sh — переключает режим в policy и проверяет состояние iptables + RCI.
# Запускать на хосте разработчика.
set -eu

KEENETIC_HOST="${KEENETIC_HOST:?Укажите KEENETIC_HOST}"
KEENETIC_USER="${KEENETIC_USER:-root}"
KEENETIC_KEY="${KEENETIC_KEY:-${HOME}/.ssh/keenetic}"
STEP="02-policy"

log()  { printf '[%s] %s\n' "${STEP}" "$*"; }
fail() { log "FAIL:${STEP}: $*"; exit 1; }
pass() { log "PASS:${STEP}"; }

ssh_cmd() {
    # shellcheck disable=SC2029
    ssh -i "${KEENETIC_KEY}" -o StrictHostKeyChecking=no \
        "${KEENETIC_USER}@${KEENETIC_HOST}" "$@"
}

# ---------- переключаем режим ----------
log "Переключаем в режим policy и перезапускаем..."
ssh_cmd "/opt/sbin/sign-craze --mode policy --restart"

# небольшая пауза, чтобы sing-box успел подняться
sleep 3

# ---------- проверка RCI policy ----------
# Запрашиваем список IP-политик изнутри роутера (RCI доступен на 127.0.0.1:79).
log "Проверяем наличие policy description=sign-craze в RCI..."
rci_out=$(ssh_cmd \
    "curl -sf http://127.0.0.1:79/rci/show/ip/policy" 2>&1) \
    || fail "RCI /show/ip/policy недоступен: ${rci_out}"

echo "${rci_out}" | grep -q "sign-craze" \
    || fail "policy с description=sign-craze не найдена в RCI: ${rci_out}"

# ---------- проверка iptables MARK ----------
log "Проверяем правило MARK в iptables mangle PREROUTING..."
ipt_out=$(ssh_cmd "/sbin/iptables -t mangle -nvL PREROUTING" 2>&1) \
    || fail "iptables недоступен: ${ipt_out}"

echo "${ipt_out}" | grep -q "MARK" \
    || fail "правило MARK не найдено в mangle PREROUTING"

# ---------- проверка статуса после смены режима ----------
log "Проверяем --status..."
status_out=$(ssh_cmd "/opt/sbin/sign-craze --status" 2>&1) || true
echo "${status_out}" | grep -qi "running" \
    || fail "--status не содержит RUNNING после смены режима"

pass
