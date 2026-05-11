# Руководство для контрибьюторов

Спасибо за интерес к sign-craze. Документ описывает, как сообщать о багах,
предлагать улучшения и оформлять pull request'ы так, чтобы изменения попали
в релиз без лишних итераций.

Перед взаимодействием с проектом ознакомьтесь с
[Кодексом поведения](CODE_OF_CONDUCT.md): он действует во всех каналах
проекта (issues, PR, wiki, Discussions).

## Содержание

- [Ограничения проекта](#ограничения-проекта)
- [Чем можно помочь](#чем-можно-помочь)
- [Сообщить о баге](#сообщить-о-баге)
- [Предложить улучшение](#предложить-улучшение)
- [Pull request: процесс](#pull-request-процесс)
- [Окружение разработки](#окружение-разработки)
- [Стиль кода и тесты](#стиль-кода-и-тесты)
- [Сообщения коммитов](#сообщения-коммитов)
- [Clean-room политика](#clean-room-политика)
- [Лицензия вклада](#лицензия-вклада)
- [Где искать ответы](#где-искать-ответы)

## Ограничения проекта

Sign-craze — Go-утилита для управления firewall/маршрутизацией на роутерах
Keenetic поверх Entware: установка sing-box / xray / mihomo, опционально `nfqws2`,
конфигурация policy/full режима, web-UI на портах `:9090`/`:9091`/`:9092`.
Проект работает в условиях встраиваемого Linux: ограниченный RAM,
отсутствие systemd/nftables, MIPS/ARM SoC.

Начиная с **v1.0.0** архитектура мульти-ядерная: единый `pkg/types.RoutingConfig`
транслируется в натив-форматы sing-box (JSON), xray (JSON) и mihomo (YAML)
через адаптеры `internal/core/*/coreadapter.go`. Web UI `:9092` унифицирован
для всех трёх ядер.

Из этого следуют технические инварианты, которые нельзя нарушать в PR:

- Бинарь sign-craze собирается **только** через podman/docker, Go на хост
  не ставится — см. `Makefile` и скилл `go-xbuild`.
- `CGO_ENABLED=0`, `GOMIPS=softfloat`, `-ldflags="-s -w"`, цель ≤ 4 МБ
  после `upx --lzma`.
- Sign-craze управляет только **своими** объектами по префиксу
  `signcraze_` и явным путям.
  Широкие операции (`iptables -F`, `ipset destroy` без имени) запрещены.
- Любое изменение публичного CLI и lifecycle команд проходит через
  обсуждение в issue: меняется контракт, на который опираются
  пользователи и скрипты установки.
- Добавление нового ядра требует обновления **четырёх** мест одновременно:
  регистрация в `internal/cli/cores.go`, адаптер `internal/core/<name>/coreadapter.go`,
  обновление `state.ValidCores` в `internal/state/state.go` и добавление
  entry в `internal/web/routingui_presets.go` (ruleSetSources).
  Тест `cores_sync_test.go` ловит drift между `ValidCores` и registry.

## Чем можно помочь

- **Баг-репорты** с диагностикой (логи, версия, модель роутера).
- **PR с тестами** — bug-fixes, мелкие улучшения, новые шаблоны
  sing-box, новые рецепты routing.
- **Документация** — wiki (`sign-craze.wiki/`), troubleshooting,
  COMPATIBILITY_MATRIX для нового железа.
- **Тестирование на реальном железе** — отчёт о работе на
  модели Keenetic, которой нет в `docs/COMPATIBILITY_MATRIX.md`.
- **Перевод/правки русскоязычных текстов** в README, wiki, CLI help.

Что **не** принимается:

- Переписывание архитектуры без issue/обсуждения.
- Косметические правки на сотни строк (форматирование, переименования
  без причины) — увеличивают diff и риск регрессии.
- Зависимости от тяжёлых фреймворков (TUI, ORM, web-фреймворки сверх
  `net/http`). Стандартная библиотека приоритетна.

## Сообщить о баге

Перед открытием issue:

1. Поиск среди открытых и закрытых issue —
   [github.com/kittylabassistant/sign-craze/issues](https://github.com/kittylabassistant/sign-craze/issues?q=is%3Aissue).
2. Чтение [`docs/TROUBLESHOOTING.md`](docs/TROUBLESHOOTING.md) и
   FAQ в wiki.
3. Запуск диагностики на роутере:

   ```sh
   sign-craze --diag           # короткая сводка состояния
   sign-craze --diag > /tmp/sign-craze-diag.txt   # сохранить полный вывод диагностики
   ```

В тело issue включите:

- **Версия sign-craze** — `sign-craze --version` или git-тег.
- **Модель Keenetic** и архитектура (`uname -m`: `mipsle`/`mips`/`arm`/`aarch64`).
- **Версия sing-box** и nfqws2 (если используется).
- **Команда**, которая воспроизводит баг (вместе с флагами).
- **Ожидаемый и фактический результат**.
- **Логи** — `/opt/var/log/sign-craze/`, релевантные строки. Перед
  публикацией удалите/замените: WAN IP, MAC, токены outbound из
  `proxy.json`, URL подписок, частные IP клиентов LAN, если это
  не ваше тестовое окружение.
- **Минимальный config** sing-box, если баг связан с шаблоном
  (с отредактированными секретами).

Issue без диагностики и команды воспроизведения закрываются с
просьбой дополнить — это не наказание, без данных баг неотличим
от misconfiguration.

### Уязвимости безопасности

**Не открывайте публичный issue.** Канал приватного раскрытия —
GitHub Security Advisories, см.
[CODE_OF_CONDUCT.md → Уязвимости безопасности](CODE_OF_CONDUCT.md#уязвимости-безопасности).

## Предложить улучшение

Открывайте issue с лейблом `enhancement` (или Discussion, если идея
сырая) и опишите:

- **Сценарий** — какой пользовательский кейс закрывается. Без
  абстрактных «было бы хорошо».
- **Альтернативы** — что уже умеет sign-craze для этой задачи и
  почему недостаточно.
- **Влияние** — затрагивает ли CLI (требует обновления
  BEHAVIOR_SPEC), runtime, web-UI, шаблон sing-box, packaging.
- **Совместимость** — не ломает ли существующие установки на
  обновлении, нужен ли migration path для конфига.

Большие фичи (новый режим routing, новое ядро proxy, новый протокол)
сначала обсуждаются в issue, потом проектируются ADR в
`docs/adr/` — только после этого PR.

## Pull request: процесс

1. **Issue или Discussion** на нетривиальные изменения. Для
   мелких фиксов (typo, документация, мелкий баг) issue
   необязателен.
2. **Fork и feature-ветка** от `main`. Имя ветки —
   `feat/<scope>-<краткое-описание>` или `fix/<scope>-...`.
3. **TDD по возможности** — сначала тест, потом реализация.
   Firewall-логика проверяется интеграционными тестами
   (`make test-integration`, Docker `--privileged`).
4. **Локальные проверки** перед push:

   ```sh
   make tidy           # go.mod в порядке
   make lint           # golangci-lint (через podman)
   make test           # unit + race
   make test-integration   # iptables/ipset (если затронуто)
   make all            # сборка под все архитектуры
   ```

5. **PR description** — что сделано, зачем, как тестировалось.
   Скриншоты для web-UI. Ссылка на issue (`Closes #N`).
6. **CI зелёный** — `ci.yml`, `firewall-integration.yml`,
   `singbox-check.yml` и релевантные integration workflows
   должны пройти. Падающий CI — блокер.
7. **Размер PR** — желательно ≤ 400 строк изменённого кода
   без учёта тестов. Большие изменения дробите на серию PR
   с явной зависимостью.
8. **Review** — мейнтейнер ревьюит, оставляет комментарии.
   Отвечайте по существу, помечайте `Resolved` только после
   фикса. Не закрывайте threads ревьюера.
9. **Merge** — squash-merge в `main`. История коммитов внутри
   feature-ветки очищается автоматически; смысловой текст
   PR попадает в release notes.

### Что блокирует merge

- Падающий CI (lint/test/build/integration).
- Регресс в `make test` или интеграционных тестах.
- Изменение публичного CLI/lifecycle без обсуждения в issue
  и обновления соответствующей документации/`--help`.
- Нарушение ownership (sign-craze трогает не свои объекты в
  iptables/ipset/файловой системе).
- Жирные зависимости без обсуждения.
- Бинарь после `make all upx` превышает 4 МБ.
- Отладочные `fmt.Println`, закомментированный код, `TODO`
  без явного follow-up issue.

## Окружение разработки

**Go на хост не ставится.** Все операции go (`build`, `test`,
`mod tidy`, `lint`) запускаются через podman/docker:

```sh
# Сборка под текущий arch:
make all

# Только mipsle:
make dist/sign-craze-mipsle

# Юнит-тесты в контейнере:
make test
```

Для интерактивной работы — скилл [`go-xbuild`](~/.claude/skills/go-xbuild/SKILL.md)
описывает обвязку. Если podman не настроен, используйте docker —
`Makefile` подхватит то, что найдёт.

Финальная валидация бинаря — **только на реальном железе**.
Smoke-test в QEMU `qemu-user-static` ловит часть проблем
(endianness, alignment), но не заменяет проверку на роутере.
Тестовые хосты сообщества — см. issue/Discussion `testing`.

## Стиль кода и тесты

- **Go 1.25+**. Используем `log/slog` (структурированные логи),
  `embed.FS` (шаблоны sing-box), `context` (отмена/таймауты).
- **Layout** — `cmd/` для main, `internal/` для приватной логики,
  `pkg/types` для core-agnostic типов (`RoutingConfig`, `CoreRenderParams`),
  `pkg/` шире — только если код реально предназначен к импорту извне.
- **Паттерн `render_rules.go`** — каждый адаптер ядра содержит отдельный файл
  `internal/core/<name>/render_rules.go` с функциями трансляции
  `types.RouteRule → <core native rule>`. Логика трансляции не смешивается
  с рендером шаблона (`render.go`). При добавлении нового матчера в
  `types.RouteRule` — обновите все три `render_rules.go` и добавьте
  table-driven тест-кейс в `render_routing_test.go` того же пакета.
- **Errors** — обязательная обёртка `fmt.Errorf("context: %w", err)`.
  Сравнение через `errors.Is`/`errors.As`. Без `panic` в runtime
  (только в `init` и при невозможной ситуации).
- **Логи** — `slog.Info`/`Warn`/`Error` с key-value. Без
  чувствительных полей (токены, ключи) в логах. Ротация — в
  `/opt/var/log/sign-craze/`.
- **CLI флаги** — POSIX: `-X` для одиночных, `--name` для длинных.
  Многосимвольный short-флаг (`-ia`) **запрещён**.
- **Concurrency** — каждая горутина имеет владельца контекста и
  путь завершения. Нет `go func() { ... }` без явного `cancel`
  или ожидания (`sync.WaitGroup`/`errgroup`).
- **Тесты**:
  - `_test.go` рядом с кодом, table-driven где разумно.
  - Интеграционные тесты iptables/ipset — помечать
    `//go:build integration` и таг `integration` в `make test-integration`.
  - Snapshot/fixture файлы — в `testdata/`.
  - **Мульти-ядерные e2e тесты** — `internal/web/routingui_multicore_test.go`.
    При изменении Routing Editor или адаптера ядра запускайте эти тесты явно:
    `go test ./internal/web/ -run TestE2E_FullFlow`.
  - **Синхронизация ValidCores** — `internal/state/cores_sync_test.go` (таг `core`).
    Обязателен при добавлении или удалении ядра из registry.
  - **Per-core CI**: тесты под конкретный бинарь-ядро помечаются тагами
    `xraycheck`, `mihomocheck`, `singboxcheck` и запускаются в отдельных
    CI-джобах (см. `.github/workflows/`). Локально:
    `go test -tags xraycheck ./internal/core/xray/... -run TestRender_Routing`.
- **Форматирование** — `gofmt` обязателен (CI ловит
  несоответствие). `goimports` приветствуется.
- **Lint** — `golangci-lint` (см. `.golangci.yml`, если есть; иначе
  defaults `make lint`). Все warnings — fix или явный `nolint`
  с объяснением.

## Сообщения коммитов

Формат — Conventional Commits с проектным набором scope'ов
(см. git log):

```plain
<type>(<scope>): <короткое описание на русском>

[опциональное тело: причина изменения, ссылки на issue]
```

**Type:** `feat` / `fix` / `docs` / `refactor` / `test` /
`chore` / `ci` / `build`.

**Scope** (наблюдаемые в репозитории):
`cli`, `singbox`, `xray`, `mihomo`, `core`, `firewall`, `dpi`, `routing`, `boot`, `install`,
`shim`, `lint`, `ci`, `web`, `state`, `geo`, `docs`.

Тело коммита — на русском, императив («добавь», «убери»,
«перенеси»), без эмодзи, без подписей сторонних AI-инструментов.
Подпись `Co-Authored-By:` допустима, если действительно был
co-author.

Первый коммит fix-а должен ссылаться на issue: `Refs #N` или
`Closes #N`.

## Clean-room политика

Sign-craze разрабатывается **clean-room** относительно XKeen и
других несовместимых по лицензии проектов: контрибьюторам
**запрещено** читать исходный код XKeen и переносить из него
структуры/имена/комментарии. Реализация пишется с нуля по
наблюдаемому поведению и публичным интерфейсам Linux/iptables/sing-box.

Если в PR обнаружится скопированный код или явное заимствование
архитектуры из закрытых/несовместимых источников — PR
закрывается, коммиты не принимаются. При сомнениях — спросите
до начала работы.

Допустимы заимствования из совместимых проектов (BSD/MIT/Apache-2)
с сохранением copyright-уведомления и записью в
[`docs/THIRD_PARTY_LICENSES.md`](docs/THIRD_PARTY_LICENSES.md).

## Лицензия вклада

Проект распространяется на условиях [BSD-3-Clause](LICENSE).
Открывая PR, вы соглашаетесь, что ваш вклад публикуется под этой
же лицензией и может быть включён в релизы sign-craze без
дополнительных ограничений.

DCO/CLA не требуется. Авторство сохраняется в git-истории и
release notes.

## Где искать ответы

Документация в репозитории (приоритет — этот порядок):

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — модули, границы,
  packet path.
- [`docs/OWNERSHIP.md`](docs/OWNERSHIP.md) — какие именно объекты
  системы создаёт и удаляет sign-craze.
- [`docs/CONSTRAINTS.md`](docs/CONSTRAINTS.md) — лимиты MIPS/RAM,
  чего нельзя.
- [`docs/COMPATIBILITY_MATRIX.md`](docs/COMPATIBILITY_MATRIX.md) —
  поддерживаемое железо/прошивки.
- [`docs/RISK_REGISTER.md`](docs/RISK_REGISTER.md) — известные
  риски и митигации.
- [`docs/TEST_MATRIX.md`](docs/TEST_MATRIX.md) — что и где
  тестируется.
- [`docs/OPERATIONS.md`](docs/OPERATIONS.md) — runtime,
  диагностика, поддержка.
- [`docs/TROUBLESHOOTING.md`](docs/TROUBLESHOOTING.md) — частые
  проблемы у пользователей.
- [`docs/adr/`](docs/adr/) — архитектурные решения.

Wiki (`sign-craze.wiki/` и онлайн) — пользовательская
документация: routing, рецепты, установка.

Каналы:

- GitHub Issues — баги, фичи.
- GitHub Discussions — вопросы, обсуждения.
- E-Mail: kittylabassistant@protonmail.com — приватные обращения
  (security, CoC violations).

Если в документации противоречие — приоритет в порядке:
`OWNERSHIP` → `ARCHITECTURE` → wiki → README.
О противоречии откройте issue с лейблом `documentation`.
