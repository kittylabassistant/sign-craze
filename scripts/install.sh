#!/bin/sh
# Установка sign-craze на роутер.
# Требования: wget, od, awk, sed, sha256sum, df (доступны в Keenetic entware).
set -eu

REPO="kittylabassistant/sign-craze"
INSTALL_DIR="/opt/sbin"
BINARY="sign-craze"
MIN_FREE_KB=30000  # 30 MB запас для бинаря и распаковки

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
    # Определить endianness: little-endian → mipsle, big-endian → mips
    if echo I | od -to2 | awk 'FNR==1{ print substr($2,6,1)}' | grep -q 1; then
      SUFFIX="mipsle"
    else
      SUFFIX="mips"
    fi
    ;;
  *)
    echo "Неподдерживаемая архитектура: $ARCH" >&2
    exit 1
    ;;
esac

# Получить последнюю версию из GitHub API
VERSION=$(wget -qO- "https://api.github.com/repos/$REPO/releases/latest" \
  | grep '"tag_name"' \
  | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

if [ -z "$VERSION" ]; then
  echo "Не удалось получить версию из GitHub API" >&2
  exit 1
fi

URL="https://github.com/$REPO/releases/download/${VERSION}/${BINARY}-${SUFFIX}"
SHA_URL="${URL}.sha256"

echo "Устанавливаю sign-craze ${VERSION} для ${SUFFIX}..."

# Проверка свободного места в /opt (safety-fixes #11)
mkdir -p "$INSTALL_DIR"
FREE_KB=$(df -k /opt | awk 'NR==2 {print $4}')
if [ -z "$FREE_KB" ] || [ "$FREE_KB" -lt "$MIN_FREE_KB" ]; then
  echo "Недостаточно места в /opt: ${FREE_KB:-0} KB < ${MIN_FREE_KB} KB" >&2
  exit 1
fi

# Скачивание во временный путь, проверка SHA256, атомарный move
TMP=$(mktemp "/tmp/${BINARY}.XXXXXX")
trap 'rm -f "$TMP" "$TMP.sha256"' EXIT

wget -O "$TMP" "$URL"

# SHA256 опционален — если файл .sha256 отсутствует на релизе, продолжаем с warning
if wget -O "$TMP.sha256" "$SHA_URL" 2>/dev/null; then
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
