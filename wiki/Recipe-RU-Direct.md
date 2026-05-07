# Рецепт: IPRU + DomainRU → direct, остальное → proxy

## Цель

Весь трафик в RU-направлении (geoip-ru + geosite-ru) идёт напрямую, минуя VPN-туннель.
Весь остальной трафик уходит через VPN-outbound. Применимо, когда нужно сохранить
полный доступ к российским сервисам без накладных расходов на туннель, сохраняя
при этом прокси-защиту для иностранных ресурсов.

## Prerequisites

- sign-craze установлен (см. INSTALL.md).
- VPN-outbound (`vless`, `trojan` или `shadowsocks`) уже описан в `routing.json`.
  Получить его tag:

  ```sh
  ssh -p 222 root@172.16.0.1 'jq -r ".outbounds[].tag" /opt/etc/sign-craze/routing.json'
  ```

- Сделать backup текущего конфига:

  ```sh
  ssh -p 222 root@172.16.0.1 'cp /opt/etc/sign-craze/routing.json /opt/etc/sign-craze/routing.json.bak'
  ```

---

## Recipe A — через WebUI :9092

Для пользователей, которым удобнее GUI.

### A.1 Поднять WebUI

```sh
ssh -p 222 root@172.16.0.1 'sign-craze --ui on'
```

### A.2 Открыть браузер

Перейти по адресу `http://172.16.0.1:9092`.
Если порт 9092 занят — fallback на 9093..9101; актуальный порт покажет:

```sh
ssh -p 222 root@172.16.0.1 'sign-craze --status'
```

### A.3 Убедиться, что VPN-outbound существует

Tab **Outbounds** → запомнить tag (например `vless-out`).

### A.4 Применить пресет ru-direct

Tab **Routing** → кнопка **"Пресеты ▾"** → выбрать `ru-direct`.
Пресет добавит правило `geoip-ru → direct` и соответствующий RuleSetRef.

<!-- TODO screenshot: routing tab presets dropdown -->

### A.5 Добавить rule set geosite-ru

Tab **Rule Sets** → кнопка **"+ Добавить"**:

| Поле             | Значение                                                                                          |
|------------------|---------------------------------------------------------------------------------------------------|
| tag              | `geosite-ru`                                                                                      |
| type             | `remote`                                                                                          |
| format           | `binary`                                                                                          |
| url              | `https://github.com/kittylabassistant/sign-craze-dat/releases/latest/download/geosite-ru.srs`               |
| download_detour  | `direct`                                                                                          |
| update_interval  | `24h0m0s`                                                                                         |

<!-- TODO screenshot: rule sets add dialog -->

### A.6 Добавить routing rule для geosite-ru

Tab **Routing** → кнопка **"+ Добавить"**:

| Поле          | Значение     |
|---------------|--------------|
| Rule Set tags | `geosite-ru` |
| Outbound      | `direct`     |

<!-- TODO screenshot: rule modal with chips -->

### A.7 Установить final

Tab **Routing** → поле **`final`** (внизу страницы) → выбрать VPN-outbound (`vless-out`).

### A.8 Предпросмотр

Кнопка **Preview** — проверить итоговый `config.json` визуально.

### A.9 Валидация

Кнопка **Validate** — должна вернуть `{"ok": true}`.

### A.10 Применить

Кнопка **Apply** — UI вернёт `{"needs_restart": true}`.

### A.11 Перезапустить sing-box

```sh
ssh -p 222 root@172.16.0.1 'sign-craze --restart && sign-craze --status'
```

---

## Recipe B — через консоль

Для скриптов, автоматизации или когда UI недоступен.

### B.1 Получить VPN tag

```sh
ssh -p 222 root@172.16.0.1 'jq -r ".outbounds[].tag" /opt/etc/sign-craze/routing.json'
```

### B.2 Применить пресет ru-direct через REST

```sh
curl -sX POST http://172.16.0.1:9092/api/presets/ru-direct/apply
```

### B.3 Добавить geosite-ru rule_set и rule

```sh
curl -sX POST http://172.16.0.1:9092/api/rule_sets \
  -H 'Content-Type: application/json' \
  -d '{"tag":"geosite-ru","type":"remote","format":"binary",
       "url":"https://github.com/kittylabassistant/sign-craze-dat/releases/latest/download/geosite-ru.srs",
       "download_detour":"direct","update_interval":"24h0m0s"}'

curl -sX POST http://172.16.0.1:9092/api/rules \
  -H 'Content-Type: application/json' \
  -d '{"rule_set":["geosite-ru"],"outbound":"direct"}'
```

### B.4 Установить final

REST не имеет PATCH для поля `final`, поэтому через jq напрямую:

```sh
ssh -p 222 root@172.16.0.1 \
  'jq ".final = \"vless-out\"" /opt/etc/sign-craze/routing.json > /tmp/r.json && mv /tmp/r.json /opt/etc/sign-craze/routing.json'
```

### B.5 Validate + Apply + Restart

```sh
curl -sX POST http://172.16.0.1:9092/api/validate
curl -sX POST http://172.16.0.1:9092/api/apply
ssh -p 222 root@172.16.0.1 'sign-craze --restart'
```

---

### B-альтернатива — прямая правка routing.json (без UI :9092)

Если WebUI не запущен, отредактировать `routing.json` вручную.
Минимальный корректный фрагмент:

```json
{
  "version": 1,
  "rules": [
    {"rule_set": ["geoip-ru"],   "outbound": "direct"},
    {"rule_set": ["geosite-ru"], "outbound": "direct"}
  ],
  "rule_sets": [
    {
      "tag": "geoip-ru",
      "type": "remote",
      "format": "binary",
      "url": "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-ru.srs",
      "download_detour": "direct",
      "update_interval": "24h0m0s"
    },
    {
      "tag": "geosite-ru",
      "type": "remote",
      "format": "binary",
      "url": "https://github.com/kittylabassistant/sign-craze-dat/releases/latest/download/geosite-ru.srs",
      "download_detour": "direct",
      "update_interval": "24h0m0s"
    }
  ],
  "final": "vless-out"
}
```

После правки файла вызвать:

```sh
ssh -p 222 root@172.16.0.1 'sign-craze --restart'
```

`sign-craze --restart` запустит `sing-box check -c` для валидации перед заменой `config.json`.

---

## Verification

**На клиенте (RPi4 172.16.0.97 или другом downstream-хосте):**

```sh
# RU: должно идти direct (без TPROXY/VPN)
curl -v https://yandex.ru 2>&1 | grep -E "Connected|SSL"

# non-RU: должно идти через VPN-outbound
curl -v https://google.com 2>&1 | grep -E "Connected|SSL"
```

**В логах sing-box на роутере:**

```sh
ssh -p 222 root@172.16.0.1 'tail -f /opt/var/log/sign-craze/sing-box.log | grep route'
```

Ожидаемый паттерн: `→ direct` для RU-трафика, `→ vless-out` (или другой VPN tag) для остального.

**Проверить статус:**

```sh
ssh -p 222 root@172.16.0.1 'sign-craze --status'
```

Должен вернуть `healthy`.

**Проверить routing rules в конфиге:**

```sh
ssh -p 222 root@172.16.0.1 'jq ".rules[] | select(.outbound==\"direct\")" /opt/etc/sign-craze/routing.json'
```

Должны присутствовать два правила: для `geoip-ru` и `geosite-ru`.

---

## Rollback

**Через WebUI:**

Tab **Routing** → выбрать строку `geosite-ru → direct` → кнопка **DELETE**.
Повторить для строки `geoip-ru → direct`.
Затем **Apply** + restart:

```sh
ssh -p 222 root@172.16.0.1 'sign-craze --restart'
```

**Через REST API:**

```sh
# Получить индексы правил
curl -s http://172.16.0.1:9092/api/rules | jq '.[].index'

# Удалить каждое правило по индексу
curl -sX DELETE http://172.16.0.1:9092/api/rules/{idx}
```

**Из резервной копии (fastest):**

```sh
ssh -p 222 root@172.16.0.1 \
  'cp /opt/etc/sign-craze/routing.json.bak /opt/etc/sign-craze/routing.json && sign-craze --restart'
```
