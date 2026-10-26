# SruNal KZ — Telegram-бот + Mini App для учебных работ

**Русский** · [English](README.md)

> **Сервис работает прямо сейчас: [@srunal_bot](https://t.me/srunal_bot)**
> Mini App открывается кнопкой в чате бота.

Telegram-first сервис для академических работ: один бот принимает заявки и
присылает уведомления, Mini App ведёт клиента по заказу (расчёт → оплата Kaspi QR →
подтверждение оператором → готовый документ в чате). Бизнес-логика (прайс, промо,
очередь генерации) живёт в Go-бэкенде; Mini App - это клиентская часть, которую
отдаёт тот же API (один origin, без CORS, авторизация только по `initData`).

## Сценарий

1. **Заказ.** Клиент жмёт «Заказать» в Mini App и проходит мастер из 4 шагов: вид
   работы и объём → темы и материалы → данные титульного листа → подтверждение.
   Цена считается на сервере (`POST /api/quote`), скидки за объём и промокоды
   применяются на сервере, а не в клиенте.
2. **Оплата.** На экране заказа - красная кнопка «Оплатить через Kaspi» (ссылка
   `qr.kaspi.kz`) и QR-код для сканирования. Клиент отмечает «Я оплатил» (с именем
   отправителя как в Kaspi), оператор сверяет поступление и подтверждает оплату.
3. **Генерация.** ИИ-агент (Cline) собирает документ по скиллу `satbayev-gost`
   (или `pptx`): Times New Roman 14, интервал 1,5, поля 30/15/20/20 мм, титульный
   лист по стандарту Satbayev University, список источников по ГОСТ 7.1-2003.
   Несколько работ = несколько отдельных документов (по одному на тему).
4. **Доставка.** Готовый файл приходит в чат бота и доступен в Mini App 30 дней.
5. **Доработка.** Клиент пишет, что поправить, оператор подтверждает и отправляет
   заказ на повторную генерацию - агент вносит правки в тот же файл.

## Что умеет панель оператора (внутри Mini App, `/operator`)

- Реестр заказов с фильтрами и поиском, проверка оплаты (подтвердить/отклонить с причиной).
- Живая консоль генерации: итерации, команды агента, токены и стоимость; лог открывается
  по подписанной ссылке в обычном браузере (без пароля и Telegram-сессии).
- Затраты на генерацию в долларах и тенге (курс USD→KZT берётся с НБ РК и обновляется сам),
  чистая выручка в статистике.
- Возврат готовой работы на доработку с правками клиента (и правками оператора).
- Отзывы (модерация), промокоды, прайс и тарифы, сервисные настройки (QR, пауза очереди).
- Несколько операторов: `/bind` в боте привязывает чат к аккаунту и не снимает привязки других.

## Интерфейс

- Языки: русский, қазақша, English (Профиль → «Язык интерфейса»).
- Тема: как в Telegram / светлая / тёмная (Профиль → «Тема»).
- Черновик заказа переживает перезапуск приложения, есть шаблоны титульных данных.

## Репозиторий

```
├── backend/            Go 1.27: API + Telegram-бот + воркер генерации (один процесс)
│   ├── cmd/api           # HTTP API, бот и воркер
│   ├── cmd/bot           # только бот и воркер
│   ├── cmd/agentcheck    # проверка CLI агента
│   ├── internal/         # api · auth (initData) · service · store · tgbot · worker
│   ├── skills/           # pptx (опубликован) · satbayev-gost (не публикуется, см. skills/README.md)
│   └── scripts/          # setup_db.sql · smoke.ps1
├── miniapp/            React + Vite + TypeScript + Tailwind Mini App
│   ├── src/telegram/      # SDK: initData, тема, safe area, MainButton/BackButton
│   ├── src/i18n/          # ru / kk / en
│   ├── src/api/           # единственный API-клиент (заголовок initData)
│   ├── src/screens/       # Home · мастер · заказ · мои заказы · отзывы · поддержка · профиль · оператор
│   └── scripts/           # dev-initdata.mjs · smoke-api.mjs · e2e-live.mjs
├── design/stitch/      экраны и спецификация дизайн-системы (референс)
├── deploy/             Dockerfile (бэкенд + собранный Mini App в одном образе), compose, TLS, runbook
├── MIGRATION_PLAN.md   план перехода на Telegram-first
├── STACK.md            как устроена система от начала до конца
├── README.md           английская версия этого документа
└── README-ru.md        русская версия (этот файл)
```

## Стек

Go 1.27 · PostgreSQL 17 · React 18 + Vite + TypeScript + Tailwind CSS 4 · Caddy (auto-HTTPS) ·
Docker Compose · Telegram Bot API (long polling) · Telegram Mini App SDK.

## Быстрый старт (разработка)

```powershell
# 1. База (один раз) - подробности в backend/README.md
$env:PGPASSWORD="<postgres-password>"
psql -h 127.0.0.1 -U postgres -f backend/scripts/setup_db.sql
psql -h 127.0.0.1 -U postgres -c "create database srunal owner srunal"

# 2. Бэкенд
cd backend
Copy-Item .env.example .env        # DATABASE_URL, TOKEN_PEPPER, токен бота
go run ./cmd/api                   # :8080 (миграции и сид - при старте)

# 3. Mini App
cd ../miniapp
npm install
npm run dev                        # :5174, /api проксируется на :8080
npm run build                      # tsc --noEmit + vite build -> dist/
```

Вне Telegram приложение показывает экран-«замок» - это by design: `initData`
существует только внутри мессенджера. Для локальной разработки
`miniapp/scripts/dev-initdata.mjs` выпускает корректно подписанный payload:

```powershell
cd miniapp
node scripts/dev-initdata.mjs            # печатает auth_date=…&user=…&hash=…
# в консоли браузера:
#   localStorage.setItem('srunal.dev.initData', '<printed string>')
#   location.reload()
```

## Деплой (production)

```powershell
cd deploy
.\push.ps1 -Server root@<host> -Domain <domain> -MiniAppShortName <short-name> -SyncAgentCredentials
```

Скрипт пакует исходники, заливает их на сервер и пересобирает контейнеры
(`db` + `api` + `caddy` с авто-HTTPS); миграции применяются при старте API.
Пошаговый runbook для чистого VDS - `deploy/VDS.md`, шаблон ssh-конфига - `deploy/SSH.md`.

## Безопасность и приватность

В репозиторий **не попадают** секреты и персональные данные клиентов:

- `.env` (токен бота, `TOKEN_PEPPER`, пароль БД), `deploy/.env` и
  `%USERPROFILE%\.cline\...\providers.json` закрыты `.gitignore`; в репозитории
  лежат только `.env.example` с пустыми значениями.
- `backend/var/` (заказы, документы, логи агента, рабочие каталоги) тоже вне git.
- Авторизация клиента - только по подписанному `initData` (HMAC токеном бота);
  токены заказов и сессий хранятся хэшами с перцем, пароли операторов - Argon2id;
  панель оператора открывается только привязанному чату.
- Персональные данные (ФИО, темы, документы, чаты) живут только в базе и в хранилище
  на сервере; в коде и в логах их нет.
- Скилл `satbayev-gost` (официальный шаблон титулки и логотип университета)
  **не публикуется** - он лежит только в рабочей копии и в вашем деплое; в репозитории
  вместо него `backend/skills/README.md` с инструкцией по установке.
- Хост, домен и учётные данные прода не публикуются (см. `deploy/SSH.md`).

## Документация

| Документ | Что внутри |
|---|---|
| `STACK.md` | вся система: бэкенд, воркер, бот, БД, деплой |
| `MIGRATION_PLAN.md` | принципы и план перехода на Telegram-first |
| `backend/README.md` | справочник бэкенда: API, команды бота, генерация, конфигурация |
| `miniapp/README.md` | Mini App: экраны, i18n, тема, локальная разработка |
| `deploy/VDS.md` | runbook: установка, операторы, агент, восстановление |
| `deploy/SSH.md` | шаблон ssh-конфига и частые операции на сервере |
| `design/stitch/` | экраны и токены дизайн-системы |

## Тесты

```powershell
cd backend
go test ./internal/...                                   # юнит-тесты
$env:TEST_DATABASE_URL="postgres://srunal:srunal@127.0.0.1:5432/srunal_test?sslmode=disable"
go test ./internal/store/...                             # интеграционные тесты БД
go run ./cmd/agentcheck                                  # проверка CLI агента
cd ../miniapp
npm run build                                            # tsc --noEmit + vite build
node scripts/smoke-api.mjs                               # HTTP-смоук сценариев Mini App
```

## Лицензия

**MIT** (см. `LICENSE`, Copyright © 2026 SruNal KZ): можно использовать, изменять и
распространять при сохранении уведомления об авторских правах. Материалы
Satbayev University (официальный шаблон титулки, логотип) в лицензию не входят и
в репозиторий не публикуются.
