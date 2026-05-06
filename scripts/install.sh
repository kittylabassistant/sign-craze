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
# Таймауты обязательны: без них зависший connect к releases висит 60-120 сек,
# и фолбэк на raw не срабатывает за разумное время.
if command -v curl >/dev/null 2>&1; then
  DL="curl -fsSL --connect-timeout 10 --max-time 60"
  DL_OUT="curl -fsSL --connect-timeout 10 --max-time 180 -o"
  DL_TYPE="curl"
elif wget --help 2>&1 | grep -q -- '--https-only\|HTTPS support'; then
  DL="wget -q -T 30 --tries=1 -O-"
  DL_OUT="wget -q -T 180 --tries=1 -O"
  DL_TYPE="wget"
else
  echo "Нужен curl или wget-ssl. Установите: opkg install curl ca-certificates" >&2
  exit 1
fi

# probe <url> — быстрая проверка доступности (5s connect / 10s total).
# Возврат 0 = доступен, !=0 = недоступен/timeout.
probe() {
  if [ "$DL_TYPE" = "curl" ]; then
    curl -sSfI --connect-timeout 5 --max-time 10 -o /dev/null "$1" 2>/dev/null
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
# Fallback: raw.githubusercontent.com — другой CDN/IP-диапазон. Помогает,
# когда releases-домен заблокирован у провайдера, а raw — проходит.
# Работает только если бинарь закоммичен в репо (по умолчанию нет — dist/ в
# .gitignore). Для override используйте SIGNCRAZE_URL=<полный URL>.
RAW_URL="https://raw.githubusercontent.com/$REPO/refs/heads/main/dist/${BINARY}-${SUFFIX}"
RAW_SHA_URL="${RAW_URL}.sha256"
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

# Pre-check: пингуем primary 10s. Если недоступен — сразу raw, без долгого
# ожидания TCP-таймаута на полной загрузке. Releases часто блокируется у
# российских провайдеров, raw.githubusercontent.com — проходит.
if ! probe "$URL"; then
  echo "WARN: $URL недоступен (probe 10s timeout), переключаюсь на raw..." >&2
  URL="$RAW_URL"
  SHA_URL="$RAW_SHA_URL"
fi

# Primary download attempt. Если внезапно упал на самой загрузке — fallback.
if ! $DL_OUT "$TMP" "$URL"; then
  if [ "$URL" != "$RAW_URL" ]; then
    echo "WARN: загрузка с $URL не удалась, пробую raw..." >&2
    if ! $DL_OUT "$TMP" "$RAW_URL"; then
      echo "Не удалось скачать бинарь ни с releases, ни с raw" >&2
      exit 1
    fi
    SHA_URL="$RAW_SHA_URL"
  else
    echo "Не удалось скачать бинарь с $URL" >&2
    exit 1
  fi
fi

# SHA256 опционален — если .sha256 отсутствует, warning
if $DL_OUT "$TMP.sha256" "$SHA_URL" 2>/dev/null; then
  EXPECTED=$(awk '{print $1}' "$TMP.sha256")
  ACTUAL=$(sha256sum "$TMP" | awk '{print $1}')
  if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "SHA256 mismatch: ожидался $EXPECTED, получен $ACTUAL" >&2
    exit 1
  fi
  echo "SHA256 OK: $ACTUAL"
else
  echo "WARN: $SHA_URL недоступен, SHA256 не проверен" >&2
fi

chmod +x "$TMP"
mv "$TMP" "$INSTALL_DIR/$BINARY"
trap - EXIT
echo "Готово: $INSTALL_DIR/$BINARY"
