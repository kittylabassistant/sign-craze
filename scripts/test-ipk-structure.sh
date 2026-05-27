#!/bin/sh
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
PASS=0
FAIL=0

check() {
    desc="$1"
    if eval "$2"; then
        echo "  OK  $desc"
        PASS=$((PASS + 1))
    else
        echo "  FAIL $desc"
        FAIL=$((FAIL + 1))
    fi
}

echo "=== Тест структуры IPK ==="

# Создаём stub-бинарь (не настоящий ELF, только для теста структуры IPK)
mkdir -p "$ROOT/dist"
printf '#!/bin/sh\necho stub\n' > "$ROOT/dist/sign-craze-mipsle"
chmod +x "$ROOT/dist/sign-craze-mipsle"

# Собираем тестовый IPK
sh "$ROOT/scripts/build-ipk.sh" mipsle v0.0.0-test

IPK="$ROOT/dist/sign-craze_0.0.0-test_mipsel-3.4.ipk"
check "IPK файл создан" "[ -f '$IPK' ]"

SHA256_FILE="$ROOT/dist/sign-craze_0.0.0-test_mipsel-3.4.ipk.sha256"
check "sha256 файл создан" "[ -f '$SHA256_FILE' ]"

# Распаковываем IPK для инспекции
TMP=$(mktemp -d)
cleanup_tmp() { rm -rf "$TMP"; }
trap cleanup_tmp EXIT

(cd "$TMP" && ar x "$IPK")

check "debian-binary существует" "[ -f '$TMP/debian-binary' ]"
check "control.tar.gz существует" "[ -f '$TMP/control.tar.gz' ]"
check "data.tar.gz существует"    "[ -f '$TMP/data.tar.gz' ]"

# Проверяем debian-binary = "2.0"
check "debian-binary == '2.0'" "[ \"\$(cat '$TMP/debian-binary')\" = '2.0' ]"

# Распаковываем control
mkdir -p "$TMP/control"
tar xzf "$TMP/control.tar.gz" -C "$TMP/control"

check "control/control существует"  "[ -f '$TMP/control/control' ]"
check "control/preinst существует"  "[ -f '$TMP/control/preinst' ]"
check "control/postinst существует" "[ -f '$TMP/control/postinst' ]"
check "control/prerm существует"    "[ -f '$TMP/control/prerm' ]"
check "control/postrm существует"   "[ -f '$TMP/control/postrm' ]"

# Проверяем поля control
check "Package: sign-craze"              "grep -q 'Package: sign-craze' '$TMP/control/control'"
check "Architecture: mipsel-3.4"        "grep -q 'Architecture: mipsel-3.4' '$TMP/control/control'"
check "Version: 0.0.0-test"             "grep -q 'Version: 0.0.0-test' '$TMP/control/control'"
check "Depends содержит ipset"          "grep -q 'Depends:.*ipset' '$TMP/control/control'"
check "Depends содержит iptables"       "grep -q 'Depends:.*iptables' '$TMP/control/control'"
check "Depends содержит iptables-mod-conntrack-extra" "grep -q 'iptables-mod-conntrack-extra' '$TMP/control/control'"
check "Depends содержит iptables-mod-tproxy"          "grep -q 'iptables-mod-tproxy' '$TMP/control/control'"
check "Depends содержит kmod-nfnetlink-queue"         "grep -q 'kmod-nfnetlink-queue' '$TMP/control/control'"
check "Depends содержит curl"           "grep -q 'Depends:.*curl' '$TMP/control/control'"
check "Нет плейсхолдера {{VERSION}}"    "! grep -q '{{VERSION}}' '$TMP/control/control'"
check "Нет плейсхолдера {{ARCH}}"       "! grep -q '{{ARCH}}' '$TMP/control/control'"
check "Нет плейсхолдера {{INSTALLED_SIZE}}" "! grep -q '{{INSTALLED_SIZE}}' '$TMP/control/control'"

# Критичный инвариант: postinst НЕ вызывает --install-auto через бинарь напрямую.
# Разрешено: echo/print с упоминанием --install-auto (подсказка пользователю).
# Запрещено: \$BIN --install-auto или sign-craze --install-auto как реальный вызов.
check "postinst не вызывает BIN --install-auto" \
    "! grep -E '^\s*\"\\\$BIN\"[[:space:]]+--install-auto' '$TMP/control/postinst'"
check "postinst содержит упоминание --install" \
    "grep -q -- '--install' '$TMP/control/postinst'"

# prerm должен содержать --stop
check "prerm содержит --stop" "grep -q -- '--stop' '$TMP/control/prerm'"

# preinst содержит логику migration guard
check "preinst содержит opkg search" "grep -q 'opkg search' '$TMP/control/preinst'"

# Проверяем права на скрипты (исполняемые)
check "preinst исполняемый"  "[ -x '$TMP/control/preinst' ]"
check "postinst исполняемый" "[ -x '$TMP/control/postinst' ]"
check "prerm исполняемый"    "[ -x '$TMP/control/prerm' ]"
check "postrm исполняемый"   "[ -x '$TMP/control/postrm' ]"

# Распаковываем data
mkdir -p "$TMP/data"
tar xzf "$TMP/data.tar.gz" -C "$TMP/data"

check "data/opt/sbin/sign-craze существует" "[ -f '$TMP/data/opt/sbin/sign-craze' ]"
check "data/opt/sbin/sign-craze исполняемый" "[ -x '$TMP/data/opt/sbin/sign-craze' ]"

# Убедимся, что data содержит ТОЛЬКО бинарь (нет лишних файлов вроде конфигов)
DATA_FILES=$(find "$TMP/data" -type f | wc -l)
check "data содержит ровно 1 файл" "[ '$DATA_FILES' -eq 1 ]"

# Чистим артефакты теста
rm -f "$ROOT/dist/sign-craze-mipsle" \
      "$ROOT/dist/sign-craze_0.0.0-test_mipsel-3.4.ipk" \
      "$ROOT/dist/sign-craze_0.0.0-test_mipsel-3.4.ipk.sha256"

echo ""
echo "Результат: $PASS проверок прошли, $FAIL упали"

if [ "$FAIL" -gt 0 ]; then
    echo "FAIL: тест структуры IPK не прошёл" >&2
    exit 1
fi

echo "OK: тест структуры IPK прошёл"
