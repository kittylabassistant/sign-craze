#!/bin/sh
# Установка sign-craze на роутер.
# Требования: wget, od, awk, sed (доступны в Keenetic entware).
set -e

REPO="kittylabassistant/sign-craze"
INSTALL_DIR="/opt/sbin"
BINARY="sign-craze"

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

echo "Устанавливаю sign-craze ${VERSION} для ${SUFFIX}..."
mkdir -p "$INSTALL_DIR"
wget -O "$INSTALL_DIR/$BINARY" "$URL"
chmod +x "$INSTALL_DIR/$BINARY"
echo "Готово: $INSTALL_DIR/$BINARY"
