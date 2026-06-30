# Установка за DPI/блокировкой GitHub-релизов

Гайд для случая когда ISP блокирует/throttle'ит `release-assets.githubusercontent.com` — типичная картина в РФ. Работающий путь и аварийный offline-сценарий.

> Для базовой установки без блокировок используйте [Installation](Installation.md).

## Симптомы

- `curl ... releases/download/...` висит 60+ секунд → `connection timed out`
- `gh-proxy.com` отдаёт 0–13 KB и обрывается / `Operation too slow`
- `ghfast.top` качает на скорости 50–100 B/s
- В логах sign-craze: `release-assets.githubusercontent.com: net/http: TLS handshake timeout`

DPI пропускает TLS clienthello, отдаёт первые 10–16 KB ответа и стопорит TCP. Direct GitHub Pages (`raw.githubusercontent.com`, Fastly CDN) — НЕ блокируется.

## Quickstart

С v0.6.2+ install.sh пробует пять каналов в порядке:

1. `github.com/releases/download/...` (direct, Azure blob — обычно блок)
2. `raw.githubusercontent.com/.../dist/sign-craze-<arch>` (Fastly — **работает**)
3. `cdn.jsdelivr.net/gh/.../dist/...` (Cloudflare — резерв)
4. `gh-proxy.com/...` (часто throttle)
5. `ghfast.top/...` (часто throttle)

Установка одной командой с роутера:

```sh
curl -fsSL https://raw.githubusercontent.com/kittylabassistant/sign-craze/refs/heads/main/scripts/install.sh | sh
```

Speed-limit watchdog: если канал отдаёт <5 KB/s в течение 30 секунд — abort, переход к следующему.

## Установка ядра + прокси

После установки бинаря — настроить ядро (sing-box) и outbound:

```sh
# URL без spx=%2F (этот параметр специфичен xray, sing-box его отвергает)
sign-craze --install-auto --core sing-box --with-dpi --proxy \
  'vless://UUID@host:port?type=grpc&encryption=none&security=reality&pbk=PBK&fp=chrome&sni=SNI&sid=SID#tag'
```

Sing-box скачает с GitHub через тот же mirror chain. Если упадёт на degraded-state (предыдущая установка не завершилась):

```sh
sign-craze --reinstall --proxy '<URL>'
```

## DPI / nfqws2

`--with-dpi` в install-auto подтягивает ipk автоматически. Включение постфактум:

```sh
sign-craze --dpi on
sign-craze --restart
```

Бинарь nfqws2 + lua/blob-ассеты (скрипты обхода, payload-blobs) скачиваются из `nfqws/nfqws2-keenetic`. С v0.6.2 install проверяет наличие ассетов, не только бинаря.

## Если ничего не скачивается — offline

Скачай артефакты на десктоп с быстрым каналом:

```sh
# с десктопа
curl -fsSLO https://raw.githubusercontent.com/kittylabassistant/sign-craze/v1.6.1/dist/sign-craze-mipsle
curl -fsSLO https://github.com/SagerNet/sing-box/releases/download/v1.13.13/sing-box-1.13.13-linux-mipsle-softfloat.tar.gz
curl -fsSLO https://github.com/nfqws/nfqws2-keenetic/releases/download/v1.2.3/nfqws2-keenetic_1.2.3_mipsel-3.4.ipk

scp -O -P 222 sign-craze-mipsle sing-box-*.tar.gz nfqws2-*.ipk root@<router>:/tmp/
```

С роутера:

```sh
# обновление бинаря
SIGNCRAZE_BIN=/tmp/sign-craze-mipsle sh /tmp/install.sh   # или скачай install.sh отдельно

# ядро + прокси
sign-craze --install-offline /tmp/sing-box-*.tar.gz --core sing-box --with-dpi \
  --proxy '<vless URL без spx>'
```

Для nfqws2 ассетов offline (если sign-craze не смог скачать .ipk):

```sh
mkdir -p /opt/var/lib/sign-craze/cache
cp /tmp/nfqws2-*.ipk /opt/var/lib/sign-craze/cache/nfqws2.ipk
WORK=$(mktemp -d /tmp/ipk.XXXXXX) && cd "$WORK"
tar -xzf /opt/var/lib/sign-craze/cache/nfqws2.ipk
mkdir -p data && tar -xzf data.tar.gz -C data
mkdir -p /opt/etc/sign-craze/blobs /opt/etc/sign-craze/lua
cp data/opt/etc/nfqws2/blobs/*.bin /opt/etc/sign-craze/blobs/
for f in data/opt/etc/nfqws2/lua/*.lua.gz; do
  out=$(basename "$f" .gz)
  gunzip -c "$f" > /opt/etc/sign-craze/lua/$out
done
sign-craze --restart
```

## Xray при жёстком DPI

Если sing-box не пробивается через DPI (VLESS/VMess детектируются провайдером), xray предлагает дополнительные транспортные опции:

- **XHTTP packet-up режимы** (`xhttpMode: packet-up`) — маскировка под обычный HTTP-загрузчик, менее детектируем чем grpc/ws
- **PQ-VLESS** (постквантовое шифрование) — дополнительный уровень обфускации
- **Vision UDP443** — проксирование UDP через TLS 443 с минимальными отличиями от HTTPS

> [!WARNING]
> **mips/mipsle (большинство Keenetic):** сборка xray v26.3.27 не стартует на старых ядрах Keenetic (3.4–4.x) — `runtime.futexwakeup ... -89` (ENOSYS); это регрессия данного релиза, прочие 26.* работают. Sign-craze на mips/mipsle всегда ставит проверенный custom-build xray v25.12.8. На arm7/arm64 upstream xray (включая v26.3.27) работает штатно.

Установка с xray:

```sh
sign-craze --install --core xray --proxy 'vless://UUID@host:443?type=xhttp&security=reality&pbk=PBK&fp=chrome&sni=SNI&sid=SID&mode=packet-up#tag'
sign-craze --start
```

Xray использует TProxy inbound (не TUN), поэтому требует `xt_TPROXY` модуль в ядре. На Keenetic с прошивкой 4.9+ модули `xt_TPROXY.ko` и `xt_socket.ko` присутствуют в `/lib/modules/` и загружаются автоматически через `insmod`. На более старых прошивках может понадобиться `--core sing-box` как fallback.

## VLESS URL: spx и выбор ядра

| Параметр | sing-box | xray | mihomo |
|---|---|---|---|
| `spx=` (spider-X, default `/`) | **НЕ** поддерживает | ✓ | ✗ |
| `type=grpc` | ✓ | deprecated в v25 (warning) | ✓ |
| `type=xhttp` | нет | ✓ | нет |
| `security=reality` | ✓ | ✓ | ✓ |

**Рекомендация:** убирай `spx=%2F` из URL — это default REALITY `/`, серверу всё равно. Тогда sign-craze автоматически выберет sing-box (TUN-mode firewall — стабильнее xray TPROXY на стоковом ядре Keenetic, у которого нет `xt_TPROXY`).

Если URL содержит `spx`, sign-craze принудительно выберет xray. **Известное ограничение:** xray policy-mode firewall в sign-craze не имеет TPROXY-правил, трафик не маршрутизируется через xray на стоковом ядре. Чтобы использовать REALITY с custom spider-path — потребуется ядро с `xt_TPROXY` (нестандартная прошивка).

## Override mirror chain

Поведение Go-кода (sing-box / nfqws2 / xray download) настраивается через env:

```sh
# в /opt/etc/init.d/S99signcraze или вручную перед командой
SIGNCRAZE_GH_MIRRORS=direct,ghfast,gh-proxy,jsdelivr sign-craze --install-auto ...
```

Имена: `direct`, `gh-proxy`, `ghfast`, `jsdelivr`. Порядок — порядок попыток.

## Диагностика mirror'ов

С роутера:

```sh
for h in raw.githubusercontent.com github.com gh-proxy.com ghfast.top \
         release-assets.githubusercontent.com; do
  printf "%-50s " "$h:"
  curl -sSI --connect-timeout 5 --max-time 8 -L "https://$h/" -o /dev/null \
    -w "code=%{http_code} time=%{time_total}\n"
done
```

Нормально работающая комбинация: raw / github / gh-proxy / ghfast возвращают `code=200`, `release-assets` показывает timeout — это **ожидаемо** в РФ, install.sh переключится на raw.

## Автор / поддержка

Issues: https://github.com/kittylabassistant/sign-craze/issues. Для багов с DPI/блокировками прикладывай вывод `sign-craze --diag` и mirror-probe выше.
