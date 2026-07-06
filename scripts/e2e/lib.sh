#!/bin/sh
# lib.sh — общая преамбула для e2e-скриптов (01-04, 99): env-парсинг
# KEENETIC_*, функции log()/fail()/pass(), обёртка ssh_cmd().
#
# Это НЕ самостоятельный скрипт: подключается через
#   . "$(dirname "$0")/lib.sh"
# в начале вызывающего скрипта, после `set -eu`. Напрямую не запускается.

KEENETIC_HOST="${KEENETIC_HOST:?Укажите KEENETIC_HOST}"
KEENETIC_USER="${KEENETIC_USER:-root}"
KEENETIC_KEY="${KEENETIC_KEY:-${HOME}/.ssh/keenetic}"

# Дополнительные опции ssh/scp. По умолчанию пусто; скрипт может задать
# СВОЁ значение ПОСЛЕ подключения lib.sh — например 04-reboot.sh добавляет
# ConnectTimeout, чтобы не ждать недоступный после reboot хост долго.
SSH_OPTS="${SSH_OPTS:-}"

# STEP — префикс лог-строк. По умолчанию берётся из имени файла-вызывающего
# скрипта (01-install.sh -> 01-install). Скрипт может задать STEP явно
# до подключения lib.sh, тогда это значение не переопределяется.
if [ -z "${STEP:-}" ]; then
    STEP="${0##*/}"
    STEP="${STEP%.sh}"
fi

log()  { printf '[%s] %s\n' "${STEP}" "$*"; }
fail() { log "FAIL:${STEP}: $*"; exit 1; }
pass() { log "PASS:${STEP}"; }

ssh_cmd() {
    # shellcheck disable=SC2029  # раскрытие на клиенте — намеренно (env хоста)
    # shellcheck disable=SC2086  # SSH_OPTS — намеренно без кавычек (список опций)
    ssh -i "${KEENETIC_KEY}" -o StrictHostKeyChecking=accept-new \
        ${SSH_OPTS} \
        "${KEENETIC_USER}@${KEENETIC_HOST}" "$@"
}
