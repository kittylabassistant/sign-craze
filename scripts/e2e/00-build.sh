#!/bin/sh
# 00-build.sh — локальная сборка бинаря под mipsle для Keenetic.
# Запускать на хосте разработчика, не на роутере.
set -eu

DIST="${DIST:-dist}"
BINARY="${DIST}/sign-craze-mipsle"
MAX_SIZE_RAW=8388608   # 8 MiB до UPX
MAX_SIZE_UPX=4194304   # 4 MiB после UPX

log() { printf '[00-build] %s\n' "$*"; }
fail() { log "FAIL:00-build: $*"; exit 1; }

# ---------- сборка ----------
log "Сборка sign-craze-mipsle..."

if make -n build-mipsle >/dev/null 2>&1; then
    make build-mipsle
else
    log "Цель build-mipsle в Makefile не найдена, сборка вручную."
    mkdir -p "${DIST}"
    CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat \
        go build -ldflags='-s -w' -o "${BINARY}" ./cmd/sign-craze
fi

# ---------- проверка существования ----------
[ -f "${BINARY}" ] || fail "бинарь ${BINARY} не создан"

# ---------- проверка размера до UPX ----------
raw_size=$(wc -c < "${BINARY}")
log "Размер до UPX: ${raw_size} байт (лимит ${MAX_SIZE_RAW})"
if [ "${raw_size}" -gt "${MAX_SIZE_RAW}" ]; then
    fail "бинарь слишком большой до UPX: ${raw_size} > ${MAX_SIZE_RAW}"
fi

# ---------- опциональный UPX ----------
if command -v upx >/dev/null 2>&1; then
    log "Применяем upx --lzma..."
    upx --lzma "${BINARY}"
    upx_size=$(wc -c < "${BINARY}")
    log "Размер после UPX: ${upx_size} байт (лимит ${MAX_SIZE_UPX})"
    if [ "${upx_size}" -gt "${MAX_SIZE_UPX}" ]; then
        fail "бинарь слишком большой после UPX: ${upx_size} > ${MAX_SIZE_UPX}"
    fi
else
    log "upx не найден, пропускаем сжатие."
fi

log "PASS:00-build  бинарь: ${BINARY}"
