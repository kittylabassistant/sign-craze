#!/bin/sh
# Установка sign-craze на роутер.
# Требования: curl ИЛИ wget-ssl, od, awk, sed, sha256sum, df.
# BusyBox wget без SSL — не поддерживается, нужен curl или entware wget-ssl.
set -eu

REPO="kittylabassistant/sign-craze"
INSTALL_DIR="/opt/sbin"
BINARY="sign-craze"
MIN_FREE_KB=30000  # 30 MB запас

# Выбрать загрузчик: curl с -k (без проверки сертификатов) или wget-ssl с --no-check-certificate
if command -v curl >/dev/null 2>&1; then
  DL="curl -kfsSL"
  DL_OUT="curl -kfsSL -o"
elif wget --help 2>&1 | grep -q -- '--no-check-certificate'; then
  DL="wget --no-check-certificate -qO-"
  DL_OUT="wget --no-check-certificate -qO"
else
  echo "Нужен curl или wget с поддержкой SSL. Установите: opkg install curl" >&2
  exit 1
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

$DL_OUT "$TMP" "$URL"

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
