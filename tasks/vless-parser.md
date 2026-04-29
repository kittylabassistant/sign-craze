# VLESS-парсер: структурный рендер для sing-box (PQ + XHTTP)

## Проблема

`internal/proxyparse/parse.go:parseVLESS` тупо копирует все query-параметры в `Outbound.Settings map[string]any`. Шаблон `tproxy.json.tmpl` рендерит это как `"settings": {...}` — формат **XRay**, а не **sing-box**. Sing-box `check` отвергает любой VLESS-конфиг (не только PQ — даже стандартный с UUID + flow).

Аналогичная проблема в `parseVMess`.

Симптомы для PQ-VLESS:
- `encryption=mlkem768x25519plus` → попадает в `settings.encryption`, sing-box ждёт на верхнем уровне.
- `type=xhttp` → конфликтует с outbound.type, должно быть в `transport.type`.
- `mldsa65Seed`/`mldsa65Verify` → должны быть в `tls.reality.*`.
- Reality params (`pbk`, `sid`, `spx`, `fp`) → должны быть в `tls.reality` + `tls.utls`.

## Цель

После фикса `--install` с любым VLESS URL (стандартный + Reality + PQ + XHTTP) должен генерировать конфиг, проходящий `sing-box check`.

## План

### 1. `pkg/types/types.go` — Outbound.MarshalJSON
Реализовать кастомный JSON-маршалинг: `Settings` мерджатся как top-level поля outbound, не заворачиваются в `"settings"`.

```go
func (o Outbound) MarshalJSON() ([]byte, error) {
    out := map[string]any{"tag": o.Tag, "type": o.Type}
    if o.Server != "" { out["server"] = o.Server }
    if o.Port != 0    { out["server_port"] = uint16(o.Port) }
    for k, v := range o.Settings { out[k] = v }
    return json.Marshal(out)
}
```

### 2. `internal/singbox/templates/tproxy.json.tmpl`
Заменить ручную сборку outbound JSON на `{{ jsonMarshal $o }}`.

### 3. `internal/proxyparse/parse.go:parseVLESS` — структурный маппинг

Известные query → правильное место в sing-box outbound:
- `uuid` (из user info) → top-level `uuid`
- `flow` → top-level `flow`
- `encryption` → top-level `encryption` (включая `mlkem768x25519plus`)
- `packetEncoding` → top-level `packet_encoding`
- `type=tcp` (default) → `network=tcp`, без transport
- `type=ws|grpc|http|xhttp|quic` → `network=tcp` + `transport={type, path, host, ...}`
- `security=tls|reality|none` → `tls.enabled` + nested
- `sni` → `tls.server_name`
- `fp` → `tls.utls.{enabled, fingerprint}`
- `pbk` → `tls.reality.{enabled, public_key}`
- `sid` → `tls.reality.short_id`
- `spx` → `tls.reality.short_id` legacy alias
- `mldsa65Seed` → `tls.reality.mldsa65_seed`
- `mldsa65Verify` → `tls.reality.mldsa65_verify`
- `path` (transport) → `transport.path`
- `host` (transport) → `transport.host`
- `serviceName` (grpc) → `transport.service_name`

Неизвестные параметры → top-level snake_case (preserve raw value).

### 4. `parseVMess` — аналогичный рефактор

Стандартный VMess: base64 JSON. Mapping: `id→uuid`, `aid→alter_id`, `net→network`, `tls→tls.enabled`, `host`, `path`. (vmess не поддерживает PQ, проще.)

### 5. Тесты
- `TestParseVLESS_PostQuantum` — `encryption=mlkem768x25519plus` + `mldsa65Verify` корректно мапятся.
- `TestParseVLESS_XHTTP` — `type=xhttp` → `transport.type=xhttp`.
- `TestParseVLESS_Reality` — `pbk/sid/fp` → структурные nested поля.
- `TestOutbound_MarshalJSON_FlatSettings` — Settings не в обёртке.
- Golden config render для VLESS Reality.

### Файлы
| Файл | Действие |
|------|---------|
| `pkg/types/types.go` | +MarshalJSON, +тест |
| `internal/proxyparse/parse.go` | переписать parseVLESS, parseVMess |
| `internal/proxyparse/parse_test.go` | новые тесты |
| `internal/singbox/templates/tproxy.json.tmpl` | упростить outbound render |
| `internal/singbox/config.go` | проверить golden tests |

### Done criteria
- `go test ./...` зелёный.
- VLESS URL с Reality+PQ+XHTTP даёт валидный sing-box config (golden test).
- `sing-box check` на сгенерированном конфиге OK (опционально — на железе).
