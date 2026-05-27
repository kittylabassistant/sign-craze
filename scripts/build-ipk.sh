#!/bin/sh
set -euo pipefail

if [ $# -ne 2 ]; then
    echo "Использование: $0 <suffix> <version>" >&2
    echo "  suffix:  arm64 | arm7 | mipsle | mips" >&2
    echo "  version: например v1.6.0" >&2
    exit 1
fi

SUFFIX="$1"
VERSION="$2"

if [ -z "$SUFFIX" ] || [ -z "$VERSION" ]; then
    echo "Ошибка: аргументы не могут быть пустыми" >&2
    exit 1
fi

ROOT=$(cd "$(dirname "$0")/.." && pwd)

# Маппинг sign-craze suffix → Entware Architecture
case "$SUFFIX" in
    arm64)   ENTWARE_ARCH="aarch64-3.10" ;;
    arm7)    ENTWARE_ARCH="armv7-3.2"    ;;
    mipsle)  ENTWARE_ARCH="mipsel-3.4"   ;;
    mips)    ENTWARE_ARCH="mips-3.4"     ;;
    *)
        echo "Ошибка: неизвестный suffix '$SUFFIX'. Допустимые: arm64, arm7, mipsle, mips" >&2
        exit 1
        ;;
esac

BINARY="$ROOT/dist/sign-craze-$SUFFIX"
if [ ! -f "$BINARY" ]; then
    echo "Ошибка: бинарь не найден: $BINARY" >&2
    echo "Запустите 'make all' перед сборкой IPK" >&2
    exit 1
fi

# opkg использует версию без leading "v"
VERSION_STRIPPED="${VERSION#v}"

OUTPUT="$ROOT/dist/sign-craze_${VERSION_STRIPPED}_${ENTWARE_ARCH}.ipk"

WORKDIR=$(mktemp -d)
cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT

# Структура data: только бинарь
mkdir -p "$WORKDIR/data/opt/sbin"
cp "$BINARY" "$WORKDIR/data/opt/sbin/sign-craze"
chmod 755 "$WORKDIR/data/opt/sbin/sign-craze"

# Структура control: скрипты из шаблонов
mkdir -p "$WORKDIR/control"
for tmpl in control preinst postinst prerm postrm; do
    cp "$ROOT/packaging/opkg/${tmpl}.tmpl" "$WORKDIR/control/${tmpl}"
done
chmod 755 "$WORKDIR/control/preinst" "$WORKDIR/control/postinst" \
           "$WORKDIR/control/prerm"  "$WORKDIR/control/postrm"

# Installed-Size в килобайтах (du -sk даёт KB)
INSTALLED_SIZE=$(du -sk "$WORKDIR/data" | awk '{print $1}')

# Подстановка плейсхолдеров в control
sed -i \
    -e "s/{{VERSION}}/$VERSION_STRIPPED/g" \
    -e "s/{{ARCH}}/$ENTWARE_ARCH/g" \
    -e "s/{{INSTALLED_SIZE}}/$INSTALLED_SIZE/g" \
    "$WORKDIR/control/control"

# Reproducibility: фиксированный mtime, детерминированный порядок, uid/gid=0
TAR_FLAGS="--mtime=@${SOURCE_DATE_EPOCH:-0} --sort=name --owner=0 --group=0 --numeric-owner"

# control.tar.gz
(cd "$WORKDIR" && tar czf control.tar.gz $TAR_FLAGS -C control .)

# data.tar.gz
(cd "$WORKDIR" && tar czf data.tar.gz $TAR_FLAGS -C data .)

# debian-binary (формат IPK)
printf '2.0\n' > "$WORKDIR/debian-binary"

# Собираем IPK через ar (стандарт: ar rc)
ar rc "$OUTPUT" "$WORKDIR/debian-binary" "$WORKDIR/control.tar.gz" "$WORKDIR/data.tar.gz"

# sha256 рядом с IPK
(cd "$(dirname "$OUTPUT")" && sha256sum "$(basename "$OUTPUT")" > "$(basename "$OUTPUT").sha256")

SHA256=$(awk '{print $1}' "$(dirname "$OUTPUT")/$(basename "$OUTPUT").sha256")
echo "IPK собран: $OUTPUT (sha256: $SHA256)"
