#!/bin/bash
# run.sh — оркестратор e2e-прогона для Keenetic.
# Запускать на хосте разработчика.
# Использование:
#   KEENETIC_HOST=192.168.1.1 ./scripts/e2e/run.sh
#   KEENETIC_HOST=192.168.1.1 ./scripts/e2e/run.sh --skip-build
#   KEENETIC_HOST=192.168.1.1 ./scripts/e2e/run.sh --from 02
set -euo pipefail

# ---------- настройки ----------
export KEENETIC_HOST="${KEENETIC_HOST:?Укажите KEENETIC_HOST (IP роутера)}"
export KEENETIC_USER="${KEENETIC_USER:-root}"
export KEENETIC_KEY="${KEENETIC_KEY:-${HOME}/.ssh/keenetic}"
export DIST="${DIST:-dist}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# ---------- разбор аргументов ----------
SKIP_BUILD=0
FROM_STEP=""

while [ $# -gt 0 ]; do
    case "$1" in
        --skip-build)
            SKIP_BUILD=1
            shift
            ;;
        --from)
            FROM_STEP="$2"
            shift 2
            ;;
        --help|-h)
            echo "Использование: KEENETIC_HOST=<ip> $0 [--skip-build] [--from <шаг>]"
            echo ""
            echo "  --skip-build     пропустить шаг 00-build.sh"
            echo "  --from <шаг>     начать с указанного шага (00, 01, 02, 03, 04, 99)"
            exit 0
            ;;
        *)
            echo "Неизвестный аргумент: $1" >&2
            exit 1
            ;;
    esac
done

# ---------- список шагов ----------
ALL_STEPS="00 01 02 03 04 99"

# ---------- вспомогательные функции ----------
declare -A STEP_RESULTS

log_header() {
    echo ""
    echo "============================================"
    echo "  sign-craze e2e — Keenetic"
    echo "  Host: ${KEENETIC_HOST}"
    echo "  User: ${KEENETIC_USER}"
    echo "  Key:  ${KEENETIC_KEY}"
    echo "============================================"
    echo ""
}

run_step() {
    local step="$1"
    local script="${SCRIPT_DIR}/${step}-"*.sh
    # shellcheck disable=SC2086
    script=$(echo ${script} | head -n1)

    if [ ! -f "${script}" ]; then
        echo "[run] SKIP:${step} — скрипт не найден: ${script}"
        STEP_RESULTS["${step}"]="SKIP"
        return 0
    fi

    echo ""
    echo "--- Шаг ${step}: $(basename "${script}") ---"

    if sh "${script}"; then
        STEP_RESULTS["${step}"]="PASS"
    else
        STEP_RESULTS["${step}"]="FAIL"
        return 1
    fi
}

print_report() {
    echo ""
    echo "============================================"
    echo "  Итоговый отчёт"
    echo "============================================"
    local overall="PASS"
    for step in ${ALL_STEPS}; do
        result="${STEP_RESULTS[${step}]:-SKIP}"
        printf "  Шаг %-5s : %s\n" "${step}" "${result}"
        if [ "${result}" = "FAIL" ]; then
            overall="FAIL"
        fi
    done
    echo "--------------------------------------------"
    echo "  Итог: ${overall}"
    echo "============================================"
    echo ""
    if [ "${overall}" = "FAIL" ]; then
        return 1
    fi
}

# ---------- основной прогон ----------
log_header

# Определяем с какого шага начинать
skip_until=""
if [ -n "${FROM_STEP}" ]; then
    skip_until="${FROM_STEP}"
fi

overall_ok=1

for step in ${ALL_STEPS}; do
    # Пропускаем шаги до --from
    if [ -n "${skip_until}" ]; then
        if [ "${step}" = "${skip_until}" ]; then
            skip_until=""
        else
            echo "[run] Пропускаем шаг ${step} (--from ${FROM_STEP})"
            STEP_RESULTS["${step}"]="SKIP"
            continue
        fi
    fi

    # --skip-build пропускает шаг 00
    if [ "${step}" = "00" ] && [ "${SKIP_BUILD}" -eq 1 ]; then
        echo "[run] Пропускаем шаг 00 (--skip-build)"
        STEP_RESULTS["${step}"]="SKIP"
        continue
    fi

    if ! run_step "${step}"; then
        overall_ok=0
        echo "[run] FAIL на шаге ${step}, прерываем прогон."
        break
    fi
done

if ! print_report; then
    exit 1
fi
