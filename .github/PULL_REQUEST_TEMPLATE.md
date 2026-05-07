<!--
Спасибо за PR. Заполните секции ниже — пустые блоки удалите. Перед открытием
PR прочитайте CONTRIBUTING.md (особенно «Pull request: процесс» и «Стиль кода
и тесты»).

Заголовок PR — в стиле Conventional Commits:
  feat(scope): ...   fix(scope): ...   refactor(scope): ...
  docs(scope): ...   test(scope): ...  chore(scope): ...
Допустимые scope: cli, singbox, xray, mihomo, firewall, dpi, routing, boot,
install, shim, lint, ci, web, state, geo, docs.
-->

## Что и зачем

<!-- 1–3 предложения: что меняется и почему это нужно. Без пересказа диффа. -->

## Связанные issue

<!-- "Closes #123" / "Refs #456". Если PR без issue — опишите контекст. -->

Closes #

## Тип изменения

<!-- Отметьте все подходящие. -->

- [ ] `fix` — исправление бага без изменения публичного поведения
- [ ] `feat` — новая возможность (CLI / web / ядро)
- [ ] `refactor` — рефакторинг без изменения поведения
- [ ] `perf` — оптимизация
- [ ] `docs` — только документация
- [ ] `test` — только тесты
- [ ] `chore` / `ci` / `lint` — инфраструктура, без рантайм-эффекта
- [ ] `security` — фикс уязвимости (см. SECURITY.md перед публичным PR)

## Затронутые подсистемы

<!-- Помогает ревью и changelog'у. -->

- [ ] cli
- [ ] install / upgrade / uninstall
- [ ] firewall (iptables/ipset, ownership `signcraze_*`)
- [ ] routing (fwmark / ip rule / table)
- [ ] sing-box / xray / mihomo
- [ ] DPI (nfqws2)
- [ ] web UI (`:9090` / `:9091` / `:9092`)
- [ ] geo (geoip / geosite)
- [ ] watchdog / autostart
- [ ] logging / diag / support-bundle
- [ ] CI / release pipeline
- [ ] документация (`docs/`, wiki, README)

## Как тестировал

<!--
Команды и условия. Укажите, что прогнал в podman, что — на железе. Если
правил firewall — добавьте, какой роутер / архитектура. Без секретов.
-->

```sh
make tidy
make lint
make test
make test-integration  # если затронут firewall/routing
make all               # multi-arch сборка проходит
```

Целевое железо:

- [ ] не требуется (чистый Go, рефакторинг, docs)
- [ ] протестировано в podman (`golang:1.25`)
- [ ] протестировано на реальном Keenetic (укажите модель и арх ниже)

Модель / арх / KeeneticOS, если применимо: `<...>`

## Чек-лист контрибьютора

- [ ] Прочитал [CONTRIBUTING.md](../blob/main/CONTRIBUTING.md) и [CODE_OF_CONDUCT.md](../blob/main/CODE_OF_CONDUCT.md)
- [ ] Заголовок PR в формате Conventional Commits с проектным `scope`
- [ ] Изменения в публичном CLI отражены в `--help` и в [`CHANGELOG.md`](../blob/main/CHANGELOG.md) (раздел `Unreleased`)
- [ ] `make tidy` без изменений в `go.mod` / `go.sum` (либо коммит включает их)
- [ ] `make lint` зелёный
- [ ] `make test` зелёный (race включён по умолчанию)
- [ ] Для firewall/routing-кода — `make test-integration` зелёный
- [ ] Новые правила firewall/ipset/chain имеют префикс `signcraze_*` (см. [`docs/OWNERSHIP.md`](../blob/main/docs/OWNERSHIP.md))
- [ ] Нет операций с чужими объектами (`-F` без префикса, `ipset destroy` чужих сетов и т. п.)
- [ ] Структурированные логи через `log/slog`, без секретов в выводе
- [ ] CGO отключён, бинарь собирается под все целевые архитектуры (`make all`)
- [ ] Размер бинаря после `upx --lzma` остался ≤ 4 МБ (если изменение его затрагивает)
- [ ] Изменения в шаблонах sing-box/xray/mihomo прогнаны через `/singbox-validate` / golden-конфиги CI
- [ ] Соблюдена clean-room политика: не использовались исходники XKeen или иных закрытых/несовместимых по лицензии проектов

## Breaking changes

<!--
Если PR ломает совместимость — опишите, что именно. CLI-флаги, формат
конфига, путь установки, протокол web UI. Дайте миграционный путь.
Если совместимость не ломается — оставьте "нет".
-->

нет

## Дополнительно

<!--
Скриншоты web UI, бенчмарки, ссылки на ADR, замечания для ревью.
Удалите блок, если пусто.
-->
