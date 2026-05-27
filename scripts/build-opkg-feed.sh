#!/bin/sh
# build-opkg-feed.sh — генерация opkg feed из .ipk файлов
#
# Использование:
#   build-opkg-feed.sh <INPUT_DIR> <OUTPUT_DIR>
#
#   INPUT_DIR  — директория с .ipk файлами (например _release_assets/)
#   OUTPUT_DIR — корневая директория feed (например entware/)
#
# Переменные окружения:
#   USIGN_KEY — путь к файлу приватного ключа .sec для usign (опционально)
#               Если не задана или usign не установлен — signing пропускается.
#
# Зависимости: ar, tar, gzip, sha256sum (все присутствуют на ubuntu-latest и Entware)
#
# Выходная структура:
#   <OUTPUT_DIR>/<arch>/sign-craze_<ver>_<arch>.ipk
#   <OUTPUT_DIR>/<arch>/Packages
#   <OUTPUT_DIR>/<arch>/Packages.gz
#   <OUTPUT_DIR>/<arch>/Packages.sig  (только если USIGN_KEY задан и usign установлен)

set -euo pipefail

# ─── Аргументы ────────────────────────────────────────────────────────────────

if [ $# -lt 2 ]; then
    echo "Использование: $0 <INPUT_DIR> <OUTPUT_DIR>" >&2
    exit 1
fi

INPUT_DIR="$1"
OUTPUT_DIR="$2"

if [ ! -d "$INPUT_DIR" ]; then
    echo "Ошибка: директория входных данных не существует: $INPUT_DIR" >&2
    exit 1
fi

mkdir -p "$OUTPUT_DIR"

# ─── Архитектуры ──────────────────────────────────────────────────────────────

ARCHES="aarch64-3.10 armv7-3.2 mipsel-3.4 mips-3.4"

# ─── Проверка наличия usign ───────────────────────────────────────────────────

USIGN_AVAILABLE=0
if command -v usign >/dev/null 2>&1; then
    USIGN_AVAILABLE=1
fi

# ─── Обработка каждой архитектуры ─────────────────────────────────────────────

for ARCH in $ARCHES; do
    ARCH_DIR="$OUTPUT_DIR/$ARCH"
    mkdir -p "$ARCH_DIR"

    # Список .ipk для этой архитектуры
    PACKAGES_CONTENT=""
    FOUND_COUNT=0

    # find с -print0 для безопасной обработки имён файлов с пробелами
    # IFS и цикл через временный файл для совместимости с POSIX sh
    TMPLIST="$(mktemp)"

    # shellcheck disable=SC2038
    find "$INPUT_DIR" -maxdepth 2 -name "sign-craze_*_${ARCH}.ipk" -print0 \
        | xargs -0 -I{} sh -c 'printf "%s\n" "$1" >> "$2"' _ {} "$TMPLIST" \
        || true

    while IFS= read -r IPK_PATH; do
        [ -z "$IPK_PATH" ] && continue

        IPK_FILENAME="$(basename "$IPK_PATH")"
        IPK_DEST="$ARCH_DIR/$IPK_FILENAME"

        echo "  Копирование: $IPK_FILENAME → $ARCH_DIR/"
        cp "$IPK_PATH" "$IPK_DEST"
        FOUND_COUNT=$((FOUND_COUNT + 1))

        # ── Извлечение control-блока из .ipk ──────────────────────────────────

        TMPDIR_EXTRACT="$(mktemp -d)"
        CONTROL_CONTENT=""

        # .ipk — это ar-архив: debian-binary, control.tar.gz, data.tar.gz
        (cd "$TMPDIR_EXTRACT" && ar x "$IPK_DEST") 2>/dev/null || {
            echo "  Предупреждение: не удалось распаковать ar-архив: $IPK_DEST" >&2
            rm -rf "$TMPDIR_EXTRACT"
            continue
        }

        # control.tar.gz может быть как control.tar.gz, так и control.tar.xz
        CONTROL_TARBALL=""
        if [ -f "$TMPDIR_EXTRACT/control.tar.gz" ]; then
            CONTROL_TARBALL="$TMPDIR_EXTRACT/control.tar.gz"
        elif [ -f "$TMPDIR_EXTRACT/control.tar.xz" ]; then
            CONTROL_TARBALL="$TMPDIR_EXTRACT/control.tar.xz"
        elif [ -f "$TMPDIR_EXTRACT/control.tar.zst" ]; then
            CONTROL_TARBALL="$TMPDIR_EXTRACT/control.tar.zst"
        fi

        if [ -z "$CONTROL_TARBALL" ]; then
            echo "  Предупреждение: control.tar.* не найден в $IPK_FILENAME" >&2
            rm -rf "$TMPDIR_EXTRACT"
            continue
        fi

        TMPDIR_CONTROL="$(mktemp -d)"
        tar -xf "$CONTROL_TARBALL" -C "$TMPDIR_CONTROL" 2>/dev/null || {
            echo "  Предупреждение: не удалось распаковать control.tar из $IPK_FILENAME" >&2
            rm -rf "$TMPDIR_EXTRACT" "$TMPDIR_CONTROL"
            continue
        }

        CONTROL_FILE=""
        if [ -f "$TMPDIR_CONTROL/control" ]; then
            CONTROL_FILE="$TMPDIR_CONTROL/control"
        elif [ -f "$TMPDIR_CONTROL/./control" ]; then
            CONTROL_FILE="$TMPDIR_CONTROL/./control"
        fi

        if [ -z "$CONTROL_FILE" ]; then
            echo "  Предупреждение: файл control не найден в $IPK_FILENAME" >&2
            rm -rf "$TMPDIR_EXTRACT" "$TMPDIR_CONTROL"
            continue
        fi

        CONTROL_CONTENT="$(cat "$CONTROL_FILE")"

        rm -rf "$TMPDIR_EXTRACT" "$TMPDIR_CONTROL"

        # ── Дополнительные поля для Packages ──────────────────────────────────

        IPK_SIZE="$(wc -c < "$IPK_DEST" | tr -d ' ')"
        IPK_SHA256="$(sha256sum "$IPK_DEST" | awk '{print $1}')"

        # Сформировать блок: содержимое control + дополнительные поля + пустая строка
        BLOCK="${CONTROL_CONTENT}
Filename: ./${IPK_FILENAME}
Size: ${IPK_SIZE}
SHA256sum: ${IPK_SHA256}
"
        PACKAGES_CONTENT="${PACKAGES_CONTENT}${BLOCK}
"

    done < "$TMPLIST"

    rm -f "$TMPLIST"

    if [ "$FOUND_COUNT" -eq 0 ]; then
        echo "  Предупреждение: .ipk не найдены для arch=${ARCH}, создаётся пустой Packages" >&2
    fi

    # ── Запись Packages ────────────────────────────────────────────────────────

    PACKAGES_FILE="$ARCH_DIR/Packages"
    printf '%s' "$PACKAGES_CONTENT" > "$PACKAGES_FILE"
    echo "  Packages: $PACKAGES_FILE (${FOUND_COUNT} пакет(ов))"

    # ── Сжатие: Packages.gz (-n = без timestamp для воспроизводимости) ─────────

    gzip -k -f -n "$PACKAGES_FILE"
    echo "  Packages.gz: ${PACKAGES_FILE}.gz"

    # ── Подпись usign (опционально) ────────────────────────────────────────────

    if [ "${USIGN_AVAILABLE}" -eq 1 ] && [ -n "${USIGN_KEY:-}" ] && [ -f "${USIGN_KEY}" ]; then
        usign -S \
            -m "${PACKAGES_FILE}.gz" \
            -s "${USIGN_KEY}" \
            -x "${ARCH_DIR}/Packages.sig"
        echo "  Packages.sig: подпись создана для arch=${ARCH}"
    else
        if [ -z "${USIGN_KEY:-}" ]; then
            echo "  Предупреждение: USIGN_KEY не задан, подпись пропущена для arch=${ARCH}" >&2
        elif [ ! -f "${USIGN_KEY}" ]; then
            echo "  Предупреждение: файл ключа не найден: ${USIGN_KEY}, подпись пропущена" >&2
        elif [ "${USIGN_AVAILABLE}" -eq 0 ]; then
            echo "  Предупреждение: usign не установлен, подпись пропущена для arch=${ARCH}" >&2
        fi
    fi

done

echo ""
echo "Feed собран для архитектур: ${ARCHES}"
echo "Выходная директория: ${OUTPUT_DIR}"
