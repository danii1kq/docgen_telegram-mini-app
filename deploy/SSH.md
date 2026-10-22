# SSH-доступ и деплой на VDS (шаблон)

> Это **публичный шаблон**: реальный хост, домен и локальные пути намеренно не
> опубликованы (они лежат в `deploy/SSH.local.md`, который в `.gitignore`).
> Подставьте свои значения.

Конфиг **без секретов**: токен бота, пароли и ключи в этот файл не входят (они лежат
в `backend/.env`, `deploy/.env` на сервере и в `%USERPROFILE%\.cline\...\providers.json`
локально - никуда не пересылать).

## 1. SSH-конфиг (положить в `~/.ssh/config` / `C:\Users\<user>\.ssh\config`)

```sshconfig
Host srunal
    HostName <vds-host>
    User root
    IdentityFile ~/.ssh/id_ed25519
    ServerAliveInterval 30
    ServerAliveCountMax 4
```

Ключ `id_ed25519` должен быть установлен на сервере (вход без пароля). Проверка:

```powershell
ssh srunal "echo ok; docker --version"
```

Если ключа на машине нет: `ssh-keygen -t ed25519` (Enter = без пароля), затем один раз
с паролем root:

```powershell
$key = (Get-Content "$env:USERPROFILE\.ssh\id_ed25519.pub" -Raw).Trim()
ssh root@<vds-host> "mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '$key' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys"
```

## 2. Что где

| Что | Значение |
|---|---|
| Сервер | `root@<vds-host>` (Ubuntu + Docker) |
| Домен / TLS | `https://<domain>` (Caddy, Let's Encrypt) |
| Каталог проекта на сервере | `/opt/srunal` |
| Compose | `/opt/srunal/deploy/docker-compose.yml`, проект `deploy` |
| Контейнеры | `deploy-api-1` (Go API + воркер + бот), `deploy-caddy-1`, `deploy-db-1` (Postgres 17) |
| API внутри | `127.0.0.1:8080`, наружу только Caddy :80/:443 |
| БД | `postgres://<user>:<password>@db:5432/<database>` |
| Mini App | отдаётся тем же API (`/`), бот `@<bot-username>` |
| Хранилище файлов | том `/data/storage` в контейнере `api` (QR: `/data/storage/settings/kaspi_qr.png`) |
| Рабочие каталоги генерации | `/data/generator/work/<номер заказа>/` (там `task.json`, `agent.log`, результат) |
| Локальный репозиторий | `<path>\to\project` |
| Скрипт деплоя | `deploy/push.ps1` (PowerShell, Windows) |

## 3. Деплой одной командой (из папки `deploy`)

```powershell
.\push.ps1 -Server root@<vds-host> -Domain <domain> -MiniAppShortName <short-name> -SyncAgentCredentials
```

Что делает: пакует исходники (без `.env`, `node_modules`, `dist`, `var`) → `scp` в
`/tmp/srunal.tgz` и скрипта `/tmp/srunal-deploy.sh` → на сервере тегирует текущий образ
`srunal-tgbot:prev` (точка отката) → `docker compose --profile tls up -d --build` →
ждёт `https://<domain>/api/health`. Сборка (Go + Vite) идёт **на сервере**, локально нужен
только PowerShell + OpenSSH. Скрипт удалённого деплоя обязан быть **LF** (CRLF ломает bash).

Полезные флаги:

| Флаг | Что делает |
|---|---|
| `-SyncAgentCredentials` | копирует `%USERPROFILE%\.cline\...\providers.json` в контейнер (ключи модели) |
| `-ForceQr` | заменить Kaspi QR в хранилище файлом `deploy/assets/kaspi_qr.png` |
| `-PaymentLink https://qr.kaspi.kz/...` | записать ссылку на оплату в `app_settings` |
| `-ForceEnv` | перезаписать `.env` (с бэкапом `.bak`) |
| `-SkipBuild` | только выложить файлы, без сборки |
| `-DryRun` | напечатать удалённый скрипт и выйти |
| `-Bootstrap -BotToken ... -BotUsername ...` | первичная установка на пустой сервер |

## 4. Частые операции через ssh

```powershell
# статус контейнеров
ssh srunal "cd /opt/srunal/deploy && docker compose ps"

# логи API/бота/воркера (всё в одном контейнере)
ssh srunal "cd /opt/srunal/deploy && docker compose logs -f --tail 100 api"

# перезапуск только API (миграции применяются при старте)
ssh srunal "cd /opt/srunal/deploy && docker compose restart api"

# SQL
ssh srunal 'cd /opt/srunal/deploy && docker compose exec -T db psql -U <user> -d <database> -c "select number,status,agent_cost_usd from orders order by created_at desc limit 5"'

# скопировать файл на сервер / внутрь контейнера
scp .\file.zip srunal:/tmp/
ssh srunal "cd /opt/srunal/deploy && docker compose cp /tmp/file.zip api:/data/storage/"

# забрать лог генерации заказа
ssh srunal "cd /opt/srunal/deploy && docker compose cp api:/data/generator/work/<номер>/agent.log /tmp/agent.log"
scp srunal:/tmp/agent.log .\agent.log

# здоровье
ssh srunal "curl -fsS https://<domain>/api/health"
```

## 5. Важно для другого чата/агента

- Не пересылать и не коммитить: `backend/.env`, `deploy/.env`,
  `%USERPROFILE%\.cline\data\settings\providers.json`, токен бота, пароль Postgres.
- Секреты уже на сервере: правки - только через `ssh` (или `-ForceEnv` с новыми значениями).
- Образ один: `srunal-tgbot:latest`; откат - `docker tag srunal-tgbot:prev srunal-tgbot:latest`
  и `docker compose up -d`.
- Внутри `api` работают и HTTP API, и воркер генерации, и Telegram-бот (long polling).
  Второй экземпляр бота на том же токене запускать нельзя - Telegram отдаст
  `409 Conflict: terminated by other getUpdates request`.
- `deploy/VDS.md` - полный runbook (первичная установка, операторы, агент, восстановление).
