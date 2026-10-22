# Деплой на VDS (Ubuntu) + DuckDNS + открытие приложения в Telegram

Пошаговый сценарий «с нуля до рабочего приложения в чате бота». Всё, что ниже, — один
контейнер: API + бот + воркер генерации + Mini App + Caddy с авто-HTTPS (Let's Encrypt).

```
Интернет ── HTTPS:443 ──► Caddy (контейнер) ──► api:8080 (контейнер)
                                   │                  ├── Mini App (React-сборка)
Telegram ── long polling ──────────┘                  ├── REST API
                                                      ├── бот + уведомления
                                                      └── воркер + cline-агент
```

---

## Быстрый путь: одна команда со своего компьютера

Всё, что нужно, делает `deploy/push.ps1`: ставит Docker на сервер, создаёт оба `.env`,
заливает код, собирает образ, поднимает стек и проверяет `https://<домен>/api/health`.

**0. DuckDNS** (один раз, в браузере): создать поддомен → вписать IP сервера → `update ip`.

**1. Деплой** (Windows, папка `deploy` проекта):

```powershell
.\push.ps1 -Server root@<IP-сервера> -Domain srunal-kz.duckdns.org `
  -Bootstrap -BotToken 123456789:AAH… -BotUsername srunal_bot
```

Пароль для внутренней БД скрипт сгенерирует сам (`-PostgresPassword '...'`, если нужен свой).

**2. BotFather** (Telegram): `/newapp` → URL `https://<домен>`, короткое имя (например `srunal`);
затем Menu Button → тот же URL и `/setcommands` → `start`, `help`, `status`.

**3. Короткое имя в конфиг** (включает кнопки-диплинки в уведомлениях) — та же команда:

```powershell
.\push.ps1 -Server root@<IP> -MiniAppShortName srunal
```

**4. Оператор и доступ агента к модели** (единственный шаг, который требует ssh):

```bash
ssh root@<IP>
cd /opt/srunal/deploy
docker compose exec api /app/bin/srunal-api -operator '@operator_almaty' -password 'СложныйПароль!2026'
```

Ключи модели — либо одной командой с компьютера (переносит локальную авторизацию cline):

```powershell
.\push.ps1 -Server root@<IP> -SyncAgentCredentials
```

…либо интерактивно на сервере (`docker compose exec -it api cline auth`), либо BYOK:
`GENERATOR_API_KEY=<ключ провайдера>` в `backend/.env` + `docker compose up -d api`.

Затем в Telegram боту: `/bind @operator_almaty СложныйПароль!2026`.
Приложение открывается кнопкой меню в чате бота, кнопкой «🚀 Открыть приложение» после `/start`
или по ссылке `https://t.me/<BotUsername>/srunal`.

> Kaspi QR (`deploy/assets/kaspi_qr.png`) `push.ps1` один раз копирует в хранилище, если QR ещё
> не загружали через панель. Заменить QR можно в любой момент: Панель оператора → Настройки →
> «Загрузить QR».

**Обновления версии - той же командой** (без `-Bootstrap`):

```powershell
.\push.ps1 -Server root@<IP> -Domain srunal-kz.duckdns.org
```

Флаги, которые чаще всего нужны: `-SyncAgentCredentials` (перенести авторизацию модели с
компьютера), `-MiniAppShortName srunal` (включить кнопки-диплинки), `-ForceEnv` (перезаписать
`.env` с бэкапом), `-ForceQr` (заменить Kaspi QR в хранилище приложения файлом из
`deploy/assets/kaspi_qr.png`), `-PaymentLink https://qr.kaspi.kz/…` (записать ссылку на оплату
в настройки), `-DryRun` (показать, что уйдёт на сервер, ничего не выполняя).

Ниже — те же шаги подробно, если что-то пошло не так или хочется делать вручную.

> Если что-то упало на сервере: у каждой команды есть `-DryRun` — она печатает и
> сохраняет в `%TEMP%\srunal-remote.sh` ровно тот скрипт, который выполнится по ssh
> (его можно проверить через `bash -n`), и ничего не выполняет. Сам скрипт передаётся
> на сервер в base64 и запускается как файл: Windows PowerShell искажает кавычки в
> длинных аргументах ssh, из-за чего раньше приезжал «поломанный» bash-скрипт.

---

## 0. Что понадобится

| Что | Где взять |
|---|---|
| VDS с Ubuntu 22.04/24.04, root-доступ по SSH | любой хостинг (Timeweb, Aeza, Hetzner…) |
| Домен | [duckdns.org](https://www.duckdns.org) — бесплатно |
| Бот | [@BotFather](https://t.me/BotFather) → `/newbot` → токен |
| Ключ модели для генерации | `opencode-go` (или BYOK-ключ провайдера) |

Открытые порты: **22** (SSH), **80** и **443** (HTTPS/ACME). Порт 8080 наружу не нужен —
он публикуется только на `127.0.0.1` для отладки с самого сервера.

---

## 1. DuckDNS: домен = IP сервера

1. Зайдите на <https://www.duckdns.org> и войдите (GitHub/Google/Reddit).
2. Создайте поддомен, например `srunal-kz` → получится `srunal-kz.duckdns.org`.
3. В поле **current ip** вставьте публичный IP вашего VDS и нажмите **update ip**.
4. Проверьте с локальной машины:

```powershell
nslookup srunal-kz.duckdns.org
# Address: <IP вашего VDS>
```

> Если IP сервера меняется, используйте `https://www.duckdns.org/update?domains=srunal-kz&token=<ваш токен>`
> в cron раз в 5 минут — у DuckDNS токен показан в личном кабинете.

---

## 2. Сервер: подготовка

```bash
ssh root@<IP сервера>

# обновление и firewall
apt update && apt upgrade -y
apt install -y ufw ca-certificates curl git
ufw allow 22/tcp && ufw allow 80/tcp && ufw allow 443/tcp
ufw --force enable

# Docker Engine + compose plugin (официальный скрипт)
curl -fsSL https://get.docker.com | sh
docker --version && docker compose version
```

---

## 3. Загрузка проекта

### Вариант A — у вас есть git-репозиторий

```bash
mkdir -p /opt && cd /opt
git clone <URL-репозитория> srunal
cd srunal
```

### Вариант B — копирование с Windows напрямую (репозитория нет)

На **локальной машине** (PowerShell, из корня проекта `srunal_tgbot_miniapp`):

```powershell
# архив без мусора (node_modules, dist, runtime-данные, секреты)
tar -czf srunal.tgz `
  --exclude=miniapp/node_modules --exclude=miniapp/dist `
  --exclude=backend/var --exclude=backend/bin `
  --exclude=".git" --exclude="*.log" `
  --exclude=backend/.env --exclude=deploy/.env `
  --exclude=design/stitch/screens/*.png --exclude=design/stitch/screens/*.html `
  .

# загрузка на сервер (подставьте свой IP)
scp srunal.tgz root@<IP сервера>:/opt/
```

На **сервере**:

```bash
cd /opt && mkdir -p srunal && tar -xzf srunal.tgz -C srunal && cd srunal
ls   # backend  deploy  miniapp  README.md …
```

---

## 4. Настройка (два файла .env)

### 4.1 Приложение: `backend/.env`

```bash
cp backend/.env.example backend/.env
nano backend/.env
```

Заполните (остальное можно оставить по умолчанию):

```ini
APP_ENV=prod
PUBLIC_BASE_URL=https://srunal-kz.duckdns.org      # ВАШ домен, https
TOKEN_PEPPER=<вывод: openssl rand -hex 32>         # сгенерируйте!
SEED_ON_START=false                                # прод: без демо-заказов
                                                   # (тарифы и настройки создадутся сами)
TELEGRAM_BOT_ENABLED=true
TELEGRAM_BOT_TOKEN=123456789:AAH...                # токен от @BotFather
TELEGRAM_BOT_USERNAME=srunal_bot                   # хэндл бота без @
TELEGRAM_MINIAPP_SHORT_NAME=srunal                 # появится на шаге 8 (BotFather)
GENERATOR_MODE=agent                               # agent = обязательно AI-агент
GENERATOR_PROVIDER=opencode-go
GENERATOR_MODEL=deepseek-v4.1-flash
GENERATOR_THINKING=high
# BYOK-вариант (необязательно, см. шаг 7):
# GENERATOR_API_KEY=...
```

Сгенерировать пеппер:

```bash
openssl rand -hex 32
```

### 4.2 Инфраструктура: `deploy/.env`

```bash
cp deploy/.env.example deploy/.env
nano deploy/.env
```

```ini
DOMAIN=srunal-kz.duckdns.org        # без https://
ACME_EMAIL=you@example.com          # необязательно: письма о продлении сертификата
POSTGRES_PASSWORD=<надёжный пароль> # задайте ДО первого запуска
API_PORT=8080
GENERATOR_MODE=agent
```

---

## 5. Запуск

Скрипт `push.ps1` делает это сам; вручную — так:

```bash
cd /opt/srunal/deploy
docker compose --profile tls up -d --build      # первый запуск: сборка образа ~5-10 мин

docker compose ps                               # api и db должны быть healthy, caddy — up
docker compose logs -f api                      # миграции, сид, "telegram bot connected"
docker compose logs -f caddy                    # "certificate obtained successfully"
```

Проверка:

```bash
curl -s http://127.0.0.1:8080/api/health        # {"status":"ok","botActive":true,...}
curl -s https://srunal-kz.duckdns.org/api/health
```

> Certbot-сертификат Caddy получает автоматически. Если в логах ошибка ACME — проверьте,
> что домен резолвится в этот сервер и порты 80/443 открыты (`ufw status`, файрвол хостинга).
> Первый выпуск может занять до минуты.

---

## 6. Первый оператор

На чистой базе (без демо-данных) операторов нет — создайте своего:

```bash
cd /opt/srunal/deploy
docker compose exec api /app/bin/srunal-api \
  -operator '@operator_almaty' -password 'СложныйПароль!2026' -operator-name 'Оператор'
```

Затем **в Telegram напишите своему боту** (от имени того чата, который будет оператором):

```
/bind @operator_almaty СложныйПароль!2026
```

Этот чат начнёт получать сообщения «💰 Оплата на проверку» с кнопками **✅ Подтвердить / ❌ Отклонить**
и получит доступ к **Панели оператора** внутри приложения (шаг 9).

> Операторов может быть несколько, и один аккаунт может быть привязан к нескольким чатам
> (таблица `operator_chats`): повторный `/bind` с другого телефона **не отвязывает** предыдущий
> чат, а уведомления о новых заказах получают все привязанные чаты (`-operator` создаёт ещё один
> аккаунт, `/bind` в третьем чате подключает его).

Альтернатива без `/bind` — вписать свой chat id в `TELEGRAM_ADMIN_CHAT_IDS` в `backend/.env`
(узнать id: отправьте боту `/id`) и перезапустить: `docker compose up -d api`.

> Хотите посмотреть, как выглядит продукт с демо-заказами (для теста/презентации)? Поставьте
> `SEED_ON_START=true` и перезапустите на **чистой** базе: приедут 10 заказов, 11 отзывов,
> 6 промокодов и оператор `@operator_almaty / Eshkere9113!xxx!`.

---

## 7. Доступ агента к модели (обязательно для генерации)

```bash
# Вариант A — интерактивный вход в контейнере (ключи сохранятся в volume app-data):
docker compose exec -it api cline auth

# Вариант B — BYOK: GENERATOR_API_KEY в backend/.env + docker compose up -d api
#   GENERATOR_API_KEY=<ключ opencode-go>   → экспортируется агенту как OPENCODE_API_KEY

# Вариант C — перенести готовую авторизацию с рабочей машины (Windows→Linux):
#   scp "$env:USERPROFILE\.cline\data\settings\providers.json" root@<IP>:/tmp/providers.json
docker compose cp /tmp/providers.json api:/data/cline/settings/providers.json
```

Проверка «вживую» — создайте тестовый заказ (шаг 9) и посмотрите лог генерации:

```bash
docker compose logs -f api | grep -E "generation|agent"
docker compose exec api ls -la /data/generator/work/
```

---

## 8. BotFather: привязать приложение к боту

1. Откройте [@BotFather](https://t.me/BotFather) → `/mybots` → выберите бота.
2. **Bot Settings → Configure Mini App** (или команда `/newapp`):
   - URL приложения: `https://srunal-kz.duckdns.org`
   - короткое имя (short name): например `srunal` → ссылка будет `t.me/<bot>/srunal`
3. Это же короткое имя пропишите в `backend/.env` → `TELEGRAM_MINIAPP_SHORT_NAME=srunal`
   и перезапустите api — тогда в уведомлениях появятся кнопки-диплинки:
   ```bash
   cd /opt/srunal/deploy && docker compose up -d api
   ```
4. **Menu Button**: Bot Settings → Menu Button → URL `https://srunal-kz.duckdns.org`
   (это кнопка у поля ввода в чате).
5. `/setcommands` → выберите бота и вставьте:
   ```
   start - Открыть приложение и справку
   help - Как это работает
   status - Статусы моих заказов
   ```
6. Проверьте: `/setmenubutton` → URL `https://srunal-kz.duckdns.org` (дублирует п. 4).

---

## 9. Как открыть приложение в Telegram

Любой из способов:

| Способ | Что сделать |
|---|---|
| Кнопка меню | В чате с ботом нажмите кнопку меню (слева у поля ввода) — приложение откроется внутри Telegram |
| `/start` | Бот ответит приветствием и кнопкой **🚀 Открыть приложение** (нужен `TELEGRAM_MINIAPP_SHORT_NAME`) |
| Прямая ссылка | `https://t.me/<bot>/<shortname>` (например `t.me/srunal_bot/srunal`) |
| Deep link в экран | `https://t.me/<bot>/<shortname>?startapp=orders` · `?startapp=new` · `?startapp=order_DG-260391` |
| Сообщение бота | После подтверждения оплаты клиент получает «✅ Оплата подтверждена» с кнопкой **👀 Следить за статусом**; готовый файл приходит документом с кнопками **⭐ Оставить отзыв / 👀 Открыть заказ** |

> Внутри Telegram `initData` подписывается токеном бота, поэтому вход в приложение
> автоматический. Если открыть домен в обычном браузере — покажется экран-заглушка
> «Приложение работает внутри Telegram».

Быстрая самопроверка полного цикла (оплата → подтверждение → генерация → файл):

```bash
cd /opt/srunal/miniapp
node scripts/e2e-live.mjs <ваш chat id> --confirm     # из логов api видно прогресс агента
```

---

## 10. Обновления

### Способ A — скрипт из репозитория (рекомендуется)

На **локальной машине** (Windows, из папки `deploy`):

```powershell
.\push.ps1 -Server root@<IP> -Domain srunal-kz.duckdns.org
```

Скрипт упакует только исходники (без `node_modules`, `dist`, `var` и **без `.env`** —
конфиги на сервере не перезаписываются), зальёт архив, распакует поверх `/opt/srunal`,
поменяет тег предыдущего образа на `srunal-tgbot:prev` (для отката), пересоберёт и
перезапустит стек, затем проверит `https://<Domain>/api/health`.

Полезные флаги: `-SkipBuild` (только залить файлы), `-KeepArchive` (оставить архив),
`-RemoteDir` (если проект лежит не в `/opt/srunal`).

### Способ B — вручную (то же самое по шагам)

```powershell
# локально
tar -czf srunal.tgz --exclude=miniapp/node_modules --exclude=miniapp/dist `
  --exclude=backend/var --exclude=backend/bin --exclude=backend/.env --exclude=deploy/.env .
scp srunal.tgz root@<IP>:/tmp/
```

```bash
# на сервере
tar -xzf /tmp/srunal.tgz -C /opt/srunal --overwrite && rm /tmp/srunal.tgz
cd /opt/srunal/deploy
docker compose --profile tls up -d --build
docker compose ps && docker compose logs --tail 30 api
```

### Способ C — через git (если выложите проект в репозиторий)

```bash
cd /opt/srunal && git pull
cd deploy && docker compose --profile tls up -d --build
```

### Способ D — если сервер слабый (сборка на своей машине)

```powershell
# локально: собрать образ (Docker Desktop должен быть в режиме linux/amd64)
docker build -f deploy/Dockerfile -t srunal-tgbot:latest .
docker save srunal-tgbot:latest | gzip > srunal-image.tgz
scp srunal-image.tgz root@<IP>:/tmp/
```

```bash
# на сервере
gunzip -c /tmp/srunal-image.tgz | docker load
cd /opt/srunal/deploy && docker compose --profile tls up -d --no-build
```

Что происходит при обновлении: `db` и volumes (`db-data`, `app-data`, `caddy-data`)
не трогаются; миграции применяются автоматически при старте api; новый код Mini App
попадает в образ на этапе сборки (`npm run build` внутри Dockerfile).

### Откат

```bash
cd /opt/srunal/deploy
docker image tag srunal-tgbot:prev srunal-tgbot:latest
docker compose --profile tls up -d --no-build --force-recreate api
```

Миграции откатывать не нужно: они добавляют столбцы (`add column if not exists`),
старый бинарь их просто игнорирует.

### Логи, бэкапы, остановка

```bash
docker compose logs -f api                # API/бот/воркер
docker compose logs -f caddy              # HTTPS

# бэкап: заказы + файлы/ключи агента
docker compose exec -T db pg_dump -U srunal srunal | gzip > /root/srunal-$(date +%F).sql.gz
docker run --rm -v deploy_app-data:/data -v /root:/backup alpine \
  tar czf /backup/app-data-$(date +%F).tar.gz -C /data .

docker compose down            # остановить (данные в volumes сохранятся)
docker compose down -v         # ВНИМАНИЕ: удалит базу и файлы
```
