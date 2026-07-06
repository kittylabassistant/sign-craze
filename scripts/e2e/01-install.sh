#!/bin/sh
# 01-install.sh — копирует бинарь на Keenetic и выполняет --install-auto.
# Запускать на хосте разработчика.
set -eu

# shellcheck source=scripts/e2e/lib.sh
. "$(dirname "$0")/lib.sh"

DIST="${DIST:-dist}"
BINARY="${DIST}/sign-craze-mipsle"
REMOTE_TMP="/tmp/sign-craze"

# ---------- проверяем бинарь ----------
[ -f "${BINARY}" ] || fail "бинарь ${BINARY} не найден — запустите 00-build.sh"

# ---------- копируем бинарь ----------
log "SCP ${BINARY} → ${KEENETIC_HOST}:${REMOTE_TMP}"
scp -i "${KEENETIC_KEY}" -o StrictHostKeyChecking=accept-new \
    "${BINARY}" "${KEENETIC_USER}@${KEENETIC_HOST}:${REMOTE_TMP}"

# ---------- установка ----------
log "Запуск --install-auto на роутере..."
ssh_cmd "chmod +x ${REMOTE_TMP} && ${REMOTE_TMP} --install-auto --core sing-box"

# ---------- проверка --status ----------
log "Проверяем --status..."
status_out=$(ssh_cmd "/opt/sbin/sign-craze --status" 2>&1) || true
echo "${status_out}" | grep -qi "running" \
    || fail "--status не содержит RUNNING: ${status_out}"

# ---------- проверка --diag ----------
log "Проверяем --diag..."
diag_out=$(ssh_cmd "/opt/sbin/sign-craze --diag" 2>&1) || true
echo "${diag_out}" | grep -qi "FAIL" \
    && fail "--diag содержит FAIL:\n${diag_out}"

# ---------- проверка init-shim ----------
log "Проверяем init-shim /opt/etc/init.d/S99signcraze..."
ssh_cmd "test -f /opt/etc/init.d/S99signcraze" \
    || fail "init-shim /opt/etc/init.d/S99signcraze отсутствует"

pass
