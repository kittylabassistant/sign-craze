#!/bin/sh
# build-wiki-site.sh — рендеринг GitHub-wiki (wiki/*.md) в статический сайт.
#
# Использование:
#   build-wiki-site.sh <WIKI_DIR> <OUT_DIR>
#
#   WIKI_DIR — директория с markdown вики (например wiki/)
#   OUT_DIR  — выходная директория сайта (например _site/)
#
# Зависимости: pandoc (рендеринг), стандартные coreutils.
#
# Артефакты сборки берутся из packaging/wiki/ (рядом со скриптом):
#   wikilinks.lua — Lua-фильтр: внутренние ссылки вики -> относительные .html
#   style.css     — оформление
#
# Особенности:
#   Home.md -> index.html (корень сайта, чинит 404 на /sign-craze/)
#   <Name>.md -> <Name>.html
#   _Sidebar.md -> навигация, встраивается в каждую страницу
#   .nojekyll создаётся в OUT_DIR (GitHub отдаёт готовый HTML без Jekyll)

set -eu

# ─── Аргументы ────────────────────────────────────────────────────────────────

if [ $# -lt 2 ]; then
    echo "Использование: $0 <WIKI_DIR> <OUT_DIR>" >&2
    exit 1
fi

WIKI_DIR="$1"
OUT_DIR="$2"

if [ ! -d "$WIKI_DIR" ]; then
    echo "Ошибка: директория вики не существует: $WIKI_DIR" >&2
    exit 1
fi

if ! command -v pandoc >/dev/null 2>&1; then
    echo "Ошибка: pandoc не установлен" >&2
    exit 1
fi

# ─── Пути к артефактам сборки (packaging/wiki/ относительно скрипта) ──────────

# shellcheck disable=SC1007  # CDPATH= — намеренный env-префикс только для cd
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ASSETS_DIR="$SCRIPT_DIR/../packaging/wiki"
LUA_FILTER="$ASSETS_DIR/wikilinks.lua"
STYLE_CSS="$ASSETS_DIR/style.css"

for f in "$LUA_FILTER" "$STYLE_CSS"; do
    if [ ! -f "$f" ]; then
        echo "Ошибка: не найден артефакт сборки: $f" >&2
        exit 1
    fi
done

SIDEBAR_MD="$WIKI_DIR/_Sidebar.md"

# ─── Подготовка OUT_DIR ───────────────────────────────────────────────────────

mkdir -p "$OUT_DIR"
cp "$STYLE_CSS" "$OUT_DIR/style.css"

# ─── Обёртки тела: sidebar + content (для двухколоночного layout) ─────────────
#
# pandoc вставляет --include-before-body сразу после <body> (перед содержимым),
# а --include-after-body — перед </body>. Оборачиваем содержимое страницы в
# <main class="content">, а слева кладём отрендеренный sidebar.

TMP_BEFORE="$(mktemp)"
TMP_AFTER="$(mktemp)"
TMP_SIDEBAR="$(mktemp)"

cleanup() { rm -f "$TMP_BEFORE" "$TMP_AFTER" "$TMP_SIDEBAR"; }
trap cleanup EXIT

# Рендер sidebar -> HTML-фрагмент (тот же фильтр ссылок)
if [ -f "$SIDEBAR_MD" ]; then
    pandoc "$SIDEBAR_MD" \
        --from gfm \
        --to html \
        --lua-filter "$LUA_FILTER" \
        --output "$TMP_SIDEBAR"
else
    echo "Предупреждение: $SIDEBAR_MD не найден, навигация будет пустой" >&2
    : > "$TMP_SIDEBAR"
fi

{
    printf '<div class="layout">\n<nav class="sidebar">\n'
    cat "$TMP_SIDEBAR"
    printf '\n</nav>\n<main class="content">\n'
} > "$TMP_BEFORE"

printf '\n</main>\n</div>\n' > "$TMP_AFTER"

# ─── Рендер каждой страницы ───────────────────────────────────────────────────

PAGE_COUNT=0

for MD in "$WIKI_DIR"/*.md; do
    [ -e "$MD" ] || continue

    BASE="$(basename "$MD" .md)"

    # _Sidebar — служебный, отдельной страницей не публикуем
    [ "$BASE" = "_Sidebar" ] && continue

    if [ "$BASE" = "Home" ]; then
        OUT_FILE="$OUT_DIR/index.html"
        PAGE_TITLE="sign-craze"
    else
        OUT_FILE="$OUT_DIR/$BASE.html"
        PAGE_TITLE="$BASE · sign-craze"
    fi

    pandoc "$MD" \
        --from gfm \
        --to html \
        --standalone \
        --lua-filter "$LUA_FILTER" \
        --include-before-body "$TMP_BEFORE" \
        --include-after-body "$TMP_AFTER" \
        --css style.css \
        --metadata "pagetitle=$PAGE_TITLE" \
        --output "$OUT_FILE"

    echo "  $MD -> $OUT_FILE"
    PAGE_COUNT=$((PAGE_COUNT + 1))
done

if [ "$PAGE_COUNT" -eq 0 ]; then
    echo "Ошибка: не найдено ни одной страницы для рендера в $WIKI_DIR" >&2
    exit 1
fi

if [ ! -f "$OUT_DIR/index.html" ]; then
    echo "Предупреждение: index.html не создан (нет Home.md) — корень сайта будет 404" >&2
fi

# ─── .nojekyll: GitHub Pages отдаёт готовый HTML без Jekyll-обработки ──────────

: > "$OUT_DIR/.nojekyll"

echo ""
echo "Сайт собран: ${PAGE_COUNT} страниц(ы)"
echo "Выходная директория: ${OUT_DIR}"
