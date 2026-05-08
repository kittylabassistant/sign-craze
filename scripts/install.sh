#!/bin/sh
# Установка sign-craze на роутер.
# Требования: curl ИЛИ wget-ssl, od, awk, sed, sha256sum, df.
# BusyBox wget без SSL — не поддерживается, нужен curl или entware wget-ssl.
set -eu

REPO="kittylabassistant/sign-craze"
INSTALL_DIR="/opt/sbin"
BINARY="sign-craze"
MIN_FREE_KB=30000  # 30 MB запас

# Выбрать загрузчик с проверкой TLS-сертификата.
# Если на роутере нет корневых CA — поставить: opkg install ca-certificates
#
# Таймауты:
#   --connect-timeout 10     — TCP+TLS handshake
#   --max-time 300            — общий потолок 5 мин на одну загрузку
#   --speed-limit 5000 --speed-time 30
#                             — abort если средняя скорость падает ниже
#                               5 KB/s в течение 30 секунд подряд. Ловит
#                               DPI-throttle: типичный паттерн —
#                               TLS clienthello проходит, отдаются первые
#                               10-16 KB, потом TCP застревает. Без этого
#                               теста --max-time 300 ждёт 5 мин впустую.
#
# wget: --read-timeout=30 — inactivity timeout (нет байт 30s → abort).
# BusyBox wget без этого флага не поддерживается (нужен entware wget-ssl).
if command -v curl >/dev/null 2>&1; then
  DL="curl -fsSL --connect-timeout 10 --max-time 60"
  DL_OUT="curl -fsSL --connect-timeout 10 --max-time 300 --speed-limit 5000 --speed-time 30 -o"
  DL_TYPE="curl"
elif wget --help 2>&1 | grep -q -- '--https-only\|HTTPS support'; then
  DL="wget -q -T 30 --tries=1 -O-"
  DL_OUT="wget -q --connect-timeout=10 --read-timeout=30 -T 300 --tries=1 -O"
  DL_TYPE="wget"
else
  echo "Нужен curl или wget-ssl. Установите: opkg install curl ca-certificates" >&2
  exit 1
fi

# probe <url> — быстрая проверка доступности (5s connect / 10s total).
# -L обязателен: github.com/releases/download/* всегда отдаёт 302 на
# release-assets.githubusercontent.com — без редиректа probe вернёт 0
# даже при заблокированном asset-домене.
probe() {
  if [ "$DL_TYPE" = "curl" ]; then
    curl -sSfIL --connect-timeout 5 --max-time 10 -o /dev/null "$1" 2>/dev/null
  else
    wget -q --spider -T 10 --tries=1 "$1" 2>/dev/null
  fi
}

# Offline-режим: SIGNCRAZE_BIN=<путь> пропускает arch detection + GitHub.
# Опционально SIGNCRAZE_SHA256=<путь> для верификации checksum.
# Опционально SIGNCRAZE_VERSION=<строка> для логирования.
if [ -n "${SIGNCRAZE_BIN:-}" ]; then
  if [ ! -f "$SIGNCRAZE_BIN" ]; then
    echo "SIGNCRAZE_BIN: файл не найден: $SIGNCRAZE_BIN" >&2
    exit 1
  fi
  VERSION="${SIGNCRAZE_VERSION:-offline}"
  echo "Offline-установка sign-craze ${VERSION}..."

  mkdir -p "$INSTALL_DIR"
  FREE_KB=$(df -k /opt | awk 'NR==2 {print $4}')
  if [ -z "$FREE_KB" ] || [ "$FREE_KB" -lt "$MIN_FREE_KB" ]; then
    echo "Недостаточно места в /opt: ${FREE_KB:-0} KB < ${MIN_FREE_KB} KB" >&2
    exit 1
  fi

  TMP=$(mktemp "/tmp/${BINARY}.XXXXXX")
  trap 'rm -f "$TMP"' EXIT
  cp "$SIGNCRAZE_BIN" "$TMP"

  if [ -n "${SIGNCRAZE_SHA256:-}" ] && [ -f "$SIGNCRAZE_SHA256" ]; then
    EXPECTED=$(awk '{print $1}' "$SIGNCRAZE_SHA256")
    ACTUAL=$(sha256sum "$TMP" | awk '{print $1}')
    if [ "$EXPECTED" != "$ACTUAL" ]; then
      echo "SHA256 mismatch: ожидался $EXPECTED, получен $ACTUAL" >&2
      exit 1
    fi
    echo "SHA256 OK: $ACTUAL"
  else
    echo "WARN: SIGNCRAZE_SHA256 не задан, checksum не проверен" >&2
  fi

  chmod +x "$TMP"
  mv "$TMP" "$INSTALL_DIR/$BINARY"
  trap - EXIT
  echo "Готово: $INSTALL_DIR/$BINARY"
  exit 0
fi

# Определить архитектуру
ARCH=$(uname -m)
case "$ARCH" in
  aarch64)
    SUFFIX="arm64"
    ;;
  armv7*)
    SUFFIX="arm7"
    ;;
  mips*)
    # Endianness через od -x + awk (BusyBox без -A/-N/-t)
    HEX=$(printf '\001\002' | od -x | awk 'NR==1{print $2; exit}')
    case "$HEX" in
      0201) SUFFIX="mipsle" ;;
      0102) SUFFIX="mips" ;;
      *) echo "Не удалось определить endianness MIPS (od вывод: $HEX)" >&2; exit 1 ;;
    esac
    ;;
  *)
    echo "Неподдерживаемая архитектура: $ARCH" >&2
    exit 1
    ;;
esac

# Получить последнюю версию из GitHub API
VERSION=$($DL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep '"tag_name"' \
  | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

if [ -z "$VERSION" ]; then
  echo "Не удалось получить версию из GitHub API" >&2
  exit 1
fi

URL="https://github.com/$REPO/releases/download/${VERSION}/${BINARY}-${SUFFIX}"
SHA_URL="${URL}.sha256"
# Mirrors: публичные reverse-proxy github releases. Помогают когда
# release-assets.githubusercontent.com заблокирован у провайдера (типично
# для российских ISP — github.com/releases/download редиректит на
# *.githubusercontent.com, который блокируется отдельно от github.com).
# Несколько mirrors потому что один из них регулярно тормозит/throttle'ит:
# 2026-05-08 наблюдалось — gh-proxy.com отдал 13KB и завис на 180s, тогда
# как ghfast.top отдал 9MB за минуту. Перебираем по списку до первого
# успеха. Override: SIGNCRAZE_URL=<полный URL>.
MIRRORS_PROXIES="https://gh-proxy.com https://ghfast.top"
if [ -n "${SIGNCRAZE_URL:-}" ]; then
  URL="$SIGNCRAZE_URL"
  SHA_URL="${SIGNCRAZE_SHA256_URL:-${URL}.sha256}"
fi

echo "Устанавливаю sign-craze ${VERSION} для ${SUFFIX}..."

# Проверка свободного места в /opt
mkdir -p "$INSTALL_DIR"
FREE_KB=$(df -k /opt | awk 'NR==2 {print $4}')
if [ -z "$FREE_KB" ] || [ "$FREE_KB" -lt "$MIN_FREE_KB" ]; then
  echo "Недостаточно места в /opt: ${FREE_KB:-0} KB < ${MIN_FREE_KB} KB" >&2
  exit 1
fi

# Скачивание во временный путь, проверка SHA256, атомарный move
TMP=$(mktemp "/tmp/${BINARY}.XXXXXX")
trap 'rm -f "$TMP" "$TMP.sha256"' EXIT

# try_download <list-of-urls> <dst> — пробует каждый URL по очереди:
# probe (5s connect / 10s total) → если живой, тянет файл. На любом сбое
# (probe fail / DL_OUT non-zero) переходит к следующему URL. Возвращает
# код последнего статуса; устанавливает CHOSEN_URL при успехе.
# Минимизирует риск зависания на одном тормозном mirror — без watchdog
# на body внутри curl, опираемся на --max-time DL_OUT (180s).
try_download() {
  _urls="$1"
  _dst="$2"
  CHOSEN_URL=""
  for u in $_urls; do
    if ! probe "$u"; then
      echo "WARN: $u недоступен (probe), пробую следующий..." >&2
      continue
    fi
    if $DL_OUT "$_dst" "$u"; then
      CHOSEN_URL="$u"
      return 0
    fi
    echo "WARN: загрузка с $u не удалась, пробую следующий..." >&2
  done
  return 1
}

# build_url_chain <github-url> — direct + все mirrors через proxy (gh-proxy
# / ghfast.top / ...) в порядке предпочтения. Direct пробуем первым: если
# release-assets.githubusercontent.com доступен — это самый быстрый путь.
build_url_chain() {
  _orig="$1"
  printf "%s" "$_orig"
  for proxy in $MIRRORS_PROXIES; do
    printf " %s/%s" "$proxy" "$_orig"
  done
}

URL_CHAIN=$(build_url_chain "$URL")
SHA_URL_CHAIN=$(build_url_chain "$SHA_URL")

if ! try_download "$URL_CHAIN" "$TMP"; then
  echo "Не удалось скачать бинарь ни с одного зеркала. Проверь DNS/блокировки или используй offline:" >&2
  echo "  scp sign-craze-${SUFFIX} router:/tmp/ && SIGNCRAZE_BIN=/tmp/sign-craze-${SUFFIX} sh install.sh" >&2
  exit 1
fi
URL="$CHOSEN_URL"

# SHA256 опционален — если .sha256 отсутствует, warning. Проходит по той
# же chain зеркал, что и бинарь (на случай если direct release-assets
# заблокирован, но проксированный URL отдал бинарь).
if try_download "$SHA_URL_CHAIN" "$TMP.sha256" 2>/dev/null; then
  EXPECTED=$(awk '{print $1}' "$TMP.sha256")
  ACTUAL=$(sha256sum "$TMP" | awk '{print $1}')
  if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "SHA256 mismatch: ожидался $EXPECTED, получен $ACTUAL" >&2
    exit 1
  fi
  echo "SHA256 OK: $ACTUAL"
else
  echo "WARN: $SHA_URL не удалось скачать ни с одного зеркала, SHA256 не проверен" >&2
fi

chmod +x "$TMP"
mv "$TMP" "$INSTALL_DIR/$BINARY"
trap - EXIT
echo "Готово: $INSTALL_DIR/$BINARY"
