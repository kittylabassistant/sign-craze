#!/bin/sh
# 03-dpi-hostlist.sh — включает DPI с hostlist и проверяет файлы конфигурации.
# Запускать на хосте разработчика.
set -eu

# shellcheck source=scripts/e2e/lib.sh
. "$(dirname "$0")/lib.sh"

DPI_TARGETS="${DPI_TARGETS:-discord.com,youtube.com,googlevideo.com}"
HOSTLIST_PATH="/opt/etc/sign-craze/dpi-hostlist.txt"
NFQWS_CONF="/opt/etc/sign-craze/nfqws2.conf"

# ---------- включаем DPI ----------
log "Включаем --dpi on..."
ssh_cmd "/opt/sbin/sign-craze --dpi on"

# ---------- задаём targets ----------
log "Задаём --dpi-targets ${DPI_TARGETS}..."
ssh_cmd "/opt/sbin/sign-craze --dpi-targets ${DPI_TARGETS}"

# ---------- проверяем dpi-hostlist.txt ----------
log "Проверяем ${HOSTLIST_PATH}..."
hostlist_out=$(ssh_cmd "cat ${HOSTLIST_PATH}" 2>&1) \
    || fail "${HOSTLIST_PATH} не существует"

# Ожидаем 3 домена из стандартного набора (может меняться через DPI_TARGETS)
domain_count=$(echo "${hostlist_out}" | grep -c '.' || true)
[ "${domain_count}" -ge 3 ] \
    || fail "${HOSTLIST_PATH} содержит менее 3 строк: ${domain_count}"

for domain in discord.com youtube.com googlevideo.com; do
    echo "${hostlist_out}" | grep -q "${domain}" \
        || fail "домен ${domain} не найден в ${HOSTLIST_PATH}"
done

# ---------- проверяем nfqws2.conf ----------
log "Проверяем ${NFQWS_CONF}..."
conf_out=$(ssh_cmd "cat ${NFQWS_CONF}" 2>&1) \
    || fail "${NFQWS_CONF} не существует"

echo "${conf_out}" | grep -q "\-\-hostlist=" \
    || fail "${NFQWS_CONF} не содержит --hostlist="

# ---------- перезапуск ----------
log "Перезапускаем sign-craze..."
ssh_cmd "/opt/sbin/sign-craze --restart"

# небольшая пауза для запуска nfqws2
sleep 3

# ---------- проверяем PID nfqws2 ----------
log "Проверяем, что nfqws2 запущен..."
nfqws_pid=$(ssh_cmd "pidof nfqws2 || cat /opt/var/run/nfqws2.pid 2>/dev/null || echo ''" 2>&1) \
    || true

if [ -z "${nfqws_pid}" ]; then
    fail "nfqws2 не запущен (pidof и pid-файл пусты)"
fi

log "nfqws2 PID: ${nfqws_pid}"
pass
