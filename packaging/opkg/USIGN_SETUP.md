# Настройка usign-ключа для opkg feed

## Зачем

Подпись `Packages.gz` позволяет opkg на роутере проверять целостность feed
перед установкой пакетов. Без подписи opkg покажет предупреждение, но всё
равно позволит установить пакет (если в `opkg.conf` не задан
`option signed_packages`). С подписью: при изменении файла пакета или индекса
опция `signed_packages` защищает от подмены.

## Генерация ключевой пары (одноразовое действие)

На локальной машине с установленным `usign` (или собранным из source):

```bash
# Вариант 1: собрать usign из source (рекомендуется — он же используется в CI)
git clone --depth=1 https://git.openwrt.org/project/usign.git
cd usign && cmake . && make

# Сгенерировать пару ключей
./usign -G \
  -s sign-craze-feed.sec \
  -p sign-craze-feed.pub \
  -c "sign-craze opkg feed"
```

Будут созданы:
- `sign-craze-feed.pub` — публичный ключ (коммитить в репо).
- `sign-craze-feed.sec` — приватный ключ (только в GitHub Secret, удалить локально).

## Установка ключей

### 1. Публичный ключ в репо

Перезаписать существующий placeholder:

```bash
cp sign-craze-feed.pub packaging/opkg/sign-craze-feed.pub
git add packaging/opkg/sign-craze-feed.pub
git commit -m "feat(opkg): установить публичный ключ usign feed"
git push
```

### 2. Приватный ключ в GitHub Secret

Закодировать в base64 и добавить как Secret:

```bash
base64 -w0 sign-craze-feed.sec
# Скопировать вывод (одна длинная строка)
```

Перейти в настройки репо:
`GitHub repo → Settings → Secrets and variables → Actions → New repository secret`

- **Name:** `USIGN_PRIVATE_KEY`
- **Value:** вставить base64-строку из предыдущего шага

### 3. Удалить локальный приватный ключ

```bash
# Безопасное удаление с перезаписью
shred -u sign-craze-feed.sec

# Если shred недоступен (macOS):
# rm -P sign-craze-feed.sec
```

## Подключение feed пользователем

После того как ключ установлен и workflow `opkg-feed.yml` отработал успешно,
пользователь подключает feed следующим образом:

```sh
# 1. Скачать публичный ключ на роутер
mkdir -p /opt/etc/opkg/keys
curl -fsSL \
  https://kittylabassistant.github.io/sign-craze/entware/sign-craze-feed.pub \
  > /opt/etc/opkg/keys/sign-craze-feed.pub

# 2. Добавить feed (заменить <arch> на свою архитектуру)
#    Поддерживаемые: aarch64-3.10, armv7-3.2, mipsel-3.4, mips-3.4
ARCH=mipsel-3.4
echo "src/gz signcraze https://kittylabassistant.github.io/sign-craze/entware/${ARCH}" \
  >> /opt/etc/opkg.conf

# 3. Включить обязательную проверку подписи (опционально, но рекомендуется)
echo "option signed_packages 'sign-craze-feed'" >> /opt/etc/opkg.conf

# 4. Обновить индекс и установить
opkg update
opkg install sign-craze
```

## Ручная проверка подписи

Если нужно убедиться, что `Packages.gz` подписан корректно:

```bash
# На машине с установленным usign:
usign -V \
  -m Packages.gz \
  -p sign-craze-feed.pub \
  -x Packages.sig
```

## Ротация ключей

При компрометации приватного ключа:

1. Сгенерировать новую ключевую пару (см. раздел «Генерация» выше).
2. Перезаписать `packaging/opkg/sign-craze-feed.pub` в репо и закоммитить.
3. Обновить GitHub Secret `USIGN_PRIVATE_KEY` новым base64-значением.
4. Запустить workflow вручную через `workflow_dispatch` (Actions → Publish opkg feed → Run workflow).
5. Уведомить пользователей через release notes — старый публичный ключ перестанет валидировать новые `Packages.sig`. Пользователям нужно будет обновить `/opt/etc/opkg/keys/sign-craze-feed.pub`.
