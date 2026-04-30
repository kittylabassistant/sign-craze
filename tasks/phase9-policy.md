# Phase 9 — `--mode policy` (Keenetic IP Policy integration)

> Финальный план после reconnaissance RCI на живом Keenetic Ultra (KN-1810,
> KeeneticOS 5.0.4). См. `~/.claude/plans/floating-petting-globe.md` для
> расширенного контекста и обоснования архитектуры.

## TL;DR

Sign-craze получает новый дефолтный режим `--mode policy`, в котором выбор
устройств для проксирования делается штатным web-UI Keenetic 5.x
(«Приоритеты подключений»). Старые `proxy/dpi/hybrid` объединяются в
`--mode full`. TUN отменён — используется существующая Keenetic policy
machinery (mark+ip rule).

## Архитектура

### Поток данных в режиме `policy`

```
[Устройство клиента, привязано к policy "sign-craze" в Keenetic UI]
      │
      ▼
[Keenetic netfilter] ── ставит fwmark = <Keenetic-присвоенный hex, e.g. 0xffffaab>
      │
      ▼
[mangle:PREROUTING -j signcraze_policy]  ←── добавляет sign-craze
      │
      ├── -m mark --mark 0xffffaab -p tcp -j TPROXY --tproxy-port 7895 --tproxy-mark 0x53
      └── -m mark --mark 0xffffaab -p udp -j TPROXY --tproxy-port 7895 --tproxy-mark 0x53
      │
      ▼
[sing-box tproxy inbound :7895] → [outbound через прокси]
      │
      ▼ (исходящие пакеты от sing-box помечены SO_MARK=0x53)
[ip rule fwmark 0x53 lookup 83] → [table 83: local 0.0.0.0/0 dev lo]
      │
      ▼
[normal default route → WAN]
```

### Поток данных в режиме `full` (legacy hybrid)

Без изменений — текущая модель: `signcraze_ipv4/v6` ipset по dst-IP +
`signcraze_full` chain + опционально NFQUEUE (DPIEnabled).

### Mark/Table generation

Keenetic присваивает policy mark и table инкрементально:

| Policy             | mark     | table4 | table6 |
| ------------------ | -------- | ------ | ------ |
| Policy0 (XKeen)    | ffffaaa  | 4096   | 4096   |
| sign-craze (новая) | ffffaab  | 4098   | 4098   |
| следующая          | ffffaac  | 4100   | 4100   |

ip rules добавляет тоже Keenetic:

```
N:   from all fwmark 0xffffaab lookup 4098
N+1: from all fwmark 0xffffaab blackhole
```

Sign-craze читает `mark` из `/rci/show/ip/policy → "sign-craze".mark`
при каждом `--start` и подставляет в свои iptables-правила.

## RCI-контракт (см. `internal/ndm`)

| Endpoint                              | Метод | Назначение                        |
| ------------------------------------- | ----- | --------------------------------- |
| `/rci/show/ip/policy`                 | GET   | Прочитать mark, table4/6          |
| `/rci/show/rc/ip/policy`              | GET   | Прочитать permit, multipath       |
| `/rci/show/ip/route`                  | GET   | Detect WAN-интерфейс              |
| `/rci/`                               | POST  | Создать/удалить policy            |
| `/rci/system/configuration/save`      | POST  | Сохранить в startup-config         |

Тело создания policy:

```json
{"ip":{"policy":{"sign-craze":{"description":"sign-craze","permit":{"interface":"GigabitEthernet1"}}}}}
```

Тело удаления:

```json
{"ip":{"policy":{"sign-craze":{"no":true}}}}
```

После CreatePolicy mark присваивается асинхронно (≤5s) — `WaitForMark`
опрашивает `GetPolicy` до появления non-empty hex.

## Реализация (по этапам)

- [x] **9.1 Types + State**
  - [x] `pkg/types/types.go`: `ModePolicy`, `ModeFull`. `LegacyModes` map для миграции. Validate.
  - [x] `internal/state/state.go`: default `ModePolicy`, поля `PolicyName`/`PolicyMark`/`PolicyTable`/`WANInterface`. Migration helper `migrateLegacyMode()` с WARN.
  - [x] Тесты: `TestMode_Validate` обновлён, `TestLegacyModes_AllMigrateToPolicy`, `TestLoad_MigratesLegacyMode`.
- [x] **9.2 NDM client (`internal/ndm`)**
  - [x] `client.go`: `Client`, `Get`, `PostJSON`, baseURL `127.0.0.1:79/rci`.
  - [x] `policy.go`: `PolicyInfo`, `GetPolicy`, `CreatePolicy`, `EnsurePolicy`, `DeletePolicy`, `WaitForMark`, `SaveConfig`. `parseMarkHex` с `ErrPolicyMarkPending`.
  - [x] `wan.go`: `DetectWANInterface` через `/show/ip/route`.
  - [x] `policy_test.go`: 10 тестов на httptest mock-сервере с фикстурами реальных дампов.
- [x] **9.3 Firewall mode dispatch**
  - [x] `internal/firewall/modes/policy.go`: `PolicyRules`, `PolicyDPIRules`, константы `PolicyChainName`/`PolicyDPIChainName`.
  - [x] `internal/firewall/applier.go`: разделение `applyPolicyMode` / `applyFullMode`. `Config.PolicyMark` + `DPIEnabled`. Remove чистит и legacy-, и policy-цепочки.
  - [x] Тесты: `TestApplier_Apply_Full_*`, `TestApplier_Apply_Policy_СоздаётСвоюЦепочку`, `TestApplier_Apply_Policy_НулевойMarkОшибка`.
- [x] **9.4 CLI integration**
  - [x] `internal/cli/cmd_mode.go`: принимает `policy|full`, маппит legacy.
  - [x] `internal/cli/cmd_lifecycle.go`: `ensureKeeneticPolicy` перед `applier.Apply` для ModePolicy.
  - [x] `internal/cli/cmd_uninstall.go`: `ndm.DeletePolicy` + `SaveConfig`.
  - [x] `internal/cli/deps.go`: `newFirewallApplier` пробрасывает `PolicyMark`/`DPIEnabled`.
- [x] **9.5 DPI на Keenetic-mark**
  - [x] Реализовано через `PolicyDPIRules` в `applier.applyPolicyMode` (фильтр `-m mark --mark <keenMark>`).
- [x] **9.6 Diag**
  - [x] `internal/diag/diag.go`: `checkKeeneticPolicy` — RCI доступен, policy найдена, mark совпадает с state.
- [x] **9.7 Spec**
  - [x] `BEHAVIOR_SPEC.md` §3 — переписан под две ветки (policy/full).
- [ ] **9.8 E2E на живом Keenetic** — требует роутер.

## Verification

См. `floating-petting-globe.md`, §Verification (10 пунктов E2E на роутере).

## Файлы

Затронутые файлы:

```
pkg/types/types.go
pkg/types/types_test.go
internal/state/state.go
internal/state/state_test.go
internal/state/managers.go
internal/firewall/applier.go
internal/firewall/applier_test.go
internal/firewall/integration_test.go
internal/firewall/modes/policy.go              ← новый
internal/cli/cmd_mode.go
internal/cli/cmd_lifecycle.go
internal/cli/cmd_uninstall.go
internal/cli/deps.go
internal/ndm/doc.go                            ← новый
internal/ndm/client.go                         ← новый
internal/ndm/policy.go                         ← новый
internal/ndm/wan.go                            ← новый
internal/ndm/policy_test.go                    ← новый
internal/diag/diag.go
internal/singbox/config.go
BEHAVIOR_SPEC.md
tasks/todo.md
tasks/phase9-policy.md
```

## Open questions / Future work

- **Watchdog**: если юзер удалит policy вручную через UI, sign-craze узнает только при следующем `--start`. Можно добавить периодический GetPolicy в фоне.
- **Несколько policy**: `sign-craze-stream`/`sign-craze-gaming` для разных outbound'ов — будущая фаза.
- **Persistence через reboot**: SaveConfig вызывается, но эмпирически проверить, что mark остаётся стабильным после `system configuration save` + reboot.
- **DPI-фильтрация по интерфейсу**: сейчас `-m mark --mark <keenMark>` ловит трафик в PREROUTING. Если NFQUEUE на интерфейсе будет работать иначе, поправить.
