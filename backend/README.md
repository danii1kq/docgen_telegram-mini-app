# SruNal KZ — backend (Go + PostgreSQL)

REST API, generation queue, admin panel backend and Telegram bot for the SruNal KZ
document service (Satbayev University). It replaces the `localStorage` mock of
`srunal_vite/src/lib/api.ts` without changing the frontend behaviour: same function
names, same JSON shapes (`camelCase`), same Russian messages, ISO 8601 UTC timestamps
and integer KZT amounts.

```
srunal_go/
├── cmd/api            # HTTP API + worker + bot in one process (main entrypoint)
├── cmd/bot            # bot + worker only (when the API runs elsewhere)
├── cmd/agentcheck     # dev probe: runs one agent invocation like the worker does
├── internal/
│   ├── api            # routes, JSON envelope, CORS, rate limits, admin session
│   ├── auth           # Argon2id, tokens, TOTP (RFC 6238)
│   ├── catalog        # products/units/statuses (mirror of catalog.ts)
│   ├── config         # env + .env loader
│   ├── domain         # API types (mirror of types.ts)
│   ├── format         # money/plural rendering (mirror of format.ts)
│   ├── pricing        # price formula (mirror of pricing.ts)
│   ├── promo          # promo checks and messages (mirror of checkPromo)
│   ├── seed           # demo data (mirror of seed.ts)
│   ├── service        # business logic shared by API, worker and bot
│   ├── store          # PostgreSQL access + migrations/*.sql (auto-applied)
│   ├── tgbot          # Telegram bot (long polling)
│   ├── validate       # form validation (mirror of validate.ts)
│   └── worker         # generation queue + real .docx writer
├── scripts/
│   ├── setup_db.sql   # creates the role/database
│   └── smoke.ps1      # end-to-end API test (76 checks)
└── .env.example       # every setting, including the Telegram bot key
```

## 1. Quick start

```powershell
# 1. PostgreSQL: create the role and the database (once)
$env:PGPASSWORD="<postgres-password>"
& "C:\Program Files\PostgreSQL\18\bin\psql.exe" -h 127.0.0.1 -U postgres -f scripts/setup_db.sql
& "C:\Program Files\PostgreSQL\18\bin\psql.exe" -h 127.0.0.1 -U postgres -c "create database srunal owner srunal"

# 2. Configuration: copy the template and edit DATABASE_URL + TOKEN_PEPPER
Copy-Item .env.example .env

# 3. Run (applies migrations, imports the demo data, starts :8080)
go run ./cmd/api

# Useful flags
go run ./cmd/api -migrate                                    # only apply migrations
go run ./cmd/api -seed                                       # import the demo data into an empty database
go run ./cmd/api -operator @operator_almaty -password 'NewSecret!2026'   # rotate an operator password
```

### Docker (Ubuntu server)

The image carries the API, the bot, the AI agent (Cline CLI), Python + pandoc and
the skills, so a payment confirmation can generate a document end to end inside
the container:

```bash
cp .env.example .env          # DATABASE_URL is overridden by compose; fill the bot token
docker compose up -d          # postgres + api, migrations and the demo seed run on start
docker compose logs -f api

# One-time agent authorization (persists in the app-data volume):
docker compose exec -it api cline auth
# ...or non-interactively / by copying an existing config:
docker compose exec api cline auth -p opencode-go -k <API_KEY> -m deepseek-v4.1-flash \
  --config /data/cline --data-dir /data/cline
# The provider config lives at /data/cline/settings/providers.json inside the volume.

# BYOK alternative (no interactive step): put the key in .env - it is exported to
# the agent under the provider's own variable (opencode-go -> OPENCODE_API_KEY,
# deepseek -> DEEPSEEK_API_KEY, openai -> OPENAI_API_KEY, override with
# GENERATOR_API_KEY_ENV):
#   GENERATOR_PROVIDER=opencode-go
#   GENERATOR_MODEL=deepseek-v4.1-flash
#   GENERATOR_API_KEY=<ключ OpenCode Go>
# Diagnose the key path with scripts/check-agent-key.sh (runs the CLI with only
# the environment variable, printing the run_result).
```

Volumes: `db-data` (PostgreSQL) and `app-data` (storage, agent work dirs, the
Cline config). `API_PORT` changes the published port. The container healthcheck
calls `/api/health`; the bot runs inside the API process.

Build the binaries:

```powershell
go build -o bin/srunal-api.exe ./cmd/api
go build -o bin/srunal-bot.exe ./cmd/bot
```

Run the API tests:

```powershell
go test ./internal/...                       # unit tests (quote, promo, validation, TOTP-free auth, docx, agent, limiter)

# Database integration tests (order lifecycle, redaction, promo rules):
$env:PGPASSWORD="<postgres-password>"
& "C:\Program Files\PostgreSQL\18\bin\psql.exe" -h 127.0.0.1 -U postgres -c "create database srunal_test owner srunal"
$env:TEST_DATABASE_URL="postgres://srunal:srunal@127.0.0.1:5432/srunal_test?sslmode=disable"
go test ./internal/store/...                 # skipped without TEST_DATABASE_URL

powershell -File scripts/smoke.ps1           # end-to-end HTTP checks (needs a running API)
```

Client authorization goes through the bot, so the smoke test cannot confirm a
login code by itself: it plays the bot's part with `psql` (writing `confirmed_at`
and `chat_id` on the challenge) and then exchanges the code for a client session.
Pass `-Db <path to psql.exe> -DbUser/-DbName/-DbPassword` when the defaults do not
match your setup; without `psql` the client and order checks are skipped with a
warning.

The smoke test creates real orders and reviews while it checks the flows (including one published
review and one confirmed payment). Point it at a throw-away database or clean up afterwards:

```sql
delete from promo_code_usages where order_number in (select number from orders where notes = 'Автотест');
delete from reviews where order_number in (select number from orders where notes = 'Автотест');
delete from orders  where notes = 'Автотест';
```

## 2. Integration with the frontend (done)

`srunal_vite/src/lib/api.ts` is a real HTTP client for this service (no mock left):

1. `VITE_API_URL=http://localhost:8080` lives in `srunal_vite/.env`; CORS already allows the dev origin.
2. Function names/signatures and the response shapes match the old mock, so the pages only changed their
   data sources.
3. Admin login: `POST /api/admin/auth/login` returns `{session, token}`; the frontend stores both
   (`srunal.admin.session`, `srunal.admin.token`), sends `Authorization: Bearer <token>` on every
   `/api/admin/*` request, re-checks the session through `GET /api/admin/me` on each panel load and drops
   it on any `401`. `POST /api/admin/auth/logout` revokes it server-side. The Telegram QR login uses
   `POST /api/admin/auth/telegram/start` + `POST /api/admin/auth/telegram/poll`.
4. Result file: the UI renders `order.resultFile.url` (a signed link) instead of the bundled
   `/demo/resultat.docx` placeholder, which remains only as a fallback.
5. Errors: the JSON envelope is mapped onto `ApiError {code, message, fields, retryAfter}`; the order form
   merges `fields` into its per-input errors, so server-side validation highlights the same inputs as the
   client-side rules.
6. Admin lists use the `{orders, counts, total}` / `{reviews, counts}` envelopes (KPI cards, chips and
   moderation tabs read true global counters); `?shape=array` is available for a plain array.
7. The public `GET /api/orders/{number}` hides the operator's private fields; the panel uses
   `GET /api/admin/orders/{number}`.
8. `POST /api/orders` sends an `Idempotency-Key`, so a double submit returns the same order and token.

## 3. Telegram bot

The bot runs **inside the API process** and uses long polling, so no public URL,
webhook or TLS certificate is needed.

### Notifications

Every state change of an order is announced in the client's chat (the templates
follow `design.md §8.2`):

| Event | Who gets it | What it carries |
|---|---|---|
| Order created | client + operators | «📄 Заказ … создан» + «💳 Перейти к оплате» (deep link) |
| «Я оплатил» | operators | amount, Kaspi sender, **✅ Подтвердить / ❌ Отклонить** buttons |
| «Я оплатил» | client | «🔎 Оплата отмечена» |
| Payment confirmed | client | «✅ Оплата подтверждена» + «👀 Следить за статусом» |
| Payment rejected | client | reason + «🔁 Я оплатил ещё раз» / «💬 Поддержка» |
| Work finished | client | **the document itself** (`sendDocument`) + «⭐ Оставить отзыв» / «👀 Открыть заказ» |
| Generation failed | operators (reason) + client («⚠️ Работа задерживается») | support button |
| Payment window elapsed | client | «⌛ Срок оплаты истёк» + «🔄 Новый заказ» |
| Order cancelled / requeued | client | support / status buttons |

Deliveries go through `telegram_outbox`, so a stopped bot never blocks the API;
result documents are stored with the file path and uploaded by the bot loop
(`sendDocument`), with a text fallback when the file TTL already elapsed.
Messages are plain text (no parse mode) and deep links are only attached when
`TELEGRAM_BOT_USERNAME` + `TELEGRAM_MINIAPP_SHORT_NAME` are configured.

### Where to paste the bot key

The token from **@BotFather** goes into `srunal_go/.env`:

```ini
# srunal_go/.env
TELEGRAM_BOT_ENABLED=true
TELEGRAM_BOT_TOKEN=123456789:AAH3xYourBotFatherTokenHere     # <= the key
TELEGRAM_BOT_USERNAME=srunal_bot                              # handle without @
TELEGRAM_ADMIN_CHAT_IDS=123456789                             # optional: operator chat ids
```

Steps:

1. In Telegram open **@BotFather** → `/newbot` → pick a name and handle.
2. BotFather answers with `Use this token to access the HTTP API: 123456789:AAH...`.
3. Paste that value into `TELEGRAM_BOT_TOKEN`, set `TELEGRAM_BOT_ENABLED=true`,
   restart `go run ./cmd/api`. The log prints `telegram bot connected username=@...`.
4. To receive operator notifications without `/bind`, put your own chat id into
   `TELEGRAM_ADMIN_CHAT_IDS`. **Как узнать свой id** (три способа):
   - отправьте боту `/id` — он ответит точным числом и готовой строкой для `.env`;
   - напишите боту `@userinfobot` — он пришлёт ваш id;
   - для группы/канала: добавьте бота в группу и отправьте там `/id`
     (id группы отрицательный: `-1001234567890`).
   Несколько адресов — через запятую. Можно не заполнять вообще: `/bind @operator_almaty <пароль>`
   привязывает чат к оператору, и уведомления начинают идти сами.

Leaving `TELEGRAM_BOT_TOKEN` empty is fine: the API starts normally, notifications
pile up in the `telegram_outbox` table and are delivered as soon as the bot is
enabled. Nothing else depends on the bot.

### What the bot does

**Clients** - the bot is the sign-in method: the site asks for a code, the client
sends `/login <code>`, and the chat becomes the client identity.

| Command / message | Result |
|---|---|
| `/login 123456` | confirms the sign-in on the site; the browser receives a client session |
| `/status` | statuses of the orders of this chat |
| `DG-260391 CLS28G` (number + access token) | optional manual linking, e.g. for an order created elsewhere |
| `/start`, `/help` | short instructions (no operator commands, no `/id`) |

Orders created in a signed-in browser are attached to the chat automatically, so
every status change arrives without any linking: payment received, payment
confirmed/rejected, generation finished (with the download link), cancellation,
expiry.

**Payment confirmation with buttons.** When a client presses «Я оплатил», the
operator gets a message with the amount and the Kaspi sender name plus two inline
buttons:

| Button | Effect |
|---|---|
| ✅ Подтвердить | `payment_claimed → queued` and the order goes straight to the AI agent (no further action needed) |
| ❌ Отклонить | `payment_claimed → payment_rejected` with the standard reason; the client receives «оплата не найдена… обратитесь в техподдержку @handle» |

The pressed message is rewritten without the buttons and shows who handled it, so
the chat stays a clean audit trail. Buttons only work for operator chats
(`/bind` or `TELEGRAM_ADMIN_CHAT_IDS`); the free-form `/reject <number> <reason>`
command is still available for custom reasons.

**Operators** - `/help op` lists the panel commands and only answers for a chat
that is bound to an operator (`/bind @operator_almaty <password>`; chats from
`TELEGRAM_ADMIN_CHAT_IDS` are trusted automatically).

| Command | Result |
|---|---|
| `/orders` | orders waiting for a payment check |
| `/order DG-260391` | order card with the Kaspi sender name |
| `/confirm DG-260391` | confirm the payment → `queued` |
| `/reject DG-260391 <reason>` | reject with a reason the client sees |
| `/cancel DG-260391`, `/retry DG-260391` | cancel / retry generation |
| `/pause`, `/resume` | stop/start the generation queue (`settings.queuePaused`) |
| `/stats` | orders per status, revenue, queue warning |
| `/login 123456` | confirm a panel sign-in on the site |

`/id` exists as a hidden helper (it prints the chat id for
`TELEGRAM_ADMIN_CHAT_IDS`) and is intentionally absent from `/start`.

Every operator action writes an `order_events` row, so the admin timeline is audited.

### Telegram login for the admin panel

The "Войти через Telegram" flow is fully server-side:

```
POST /api/admin/auth/telegram/start   -> {code, expiresAt, deepLink, instruction}
   (the operator sends /login <code> to the bot, or opens the deep link)
POST /api/admin/auth/telegram/poll    -> {pending: true}
                                      -> {session, token} once the bot confirms
```

### Client sign-in (required before ordering)

The same mechanism issues a **client** session for chats that are not operators:
the chat id is the client identity and orders created with the session attach to
it automatically.

```
POST /api/auth/telegram/start   -> {code, expiresAt, botUrl, deepLink}
POST /api/auth/telegram/poll    -> {pending: true}
                                -> {session: {chatId, username}, token,
                                    operator?: {session, token}}   # bound operator chat
GET  /api/auth/me               -> the client session of the bearer token
POST /api/auth/logout           -> revoke it
```

`POST /api/orders` requires such a session (`Authorization: Bearer <client token>`,
otherwise `401`), and stores `telegram_chat_id` + `contact` on the order, so the
operator always has a reachable contact and the client gets the status pushes.

## 4. API reference

Base path `/api`, JSON only. Errors:

```json
{ "error": { "code": "validation_failed", "message": "Проверьте выделенные поля",
             "fields": { "topics": "Укажите тему работы (минимум 3 символа)" } } }
```

Codes: `validation_failed`, `not_found`, `maintenance`, `rate_limited`,
`invalid_promo`, `unauthorized`, `conflict`, `internal`.

### Public

| Method & path | Notes |
|---|---|
| `GET /api/health` | `{status, database, version, botActive}` |
| `GET /api/products` | catalogue (names, unit words, ranges) |
| `POST /api/quote` | `{product, works, units, promo?}` → `Quote` (p95 < 1 ms) |
| `POST /api/promos/check` | `{code, product, units, works?, isFirstOrder, subtotal?}` → `{ok, promo}` or `{ok, code, reason}` |
| `POST /api/auth/telegram/start`, `/api/auth/telegram/poll`, `GET /api/auth/me`, `POST /api/auth/logout` | client sign-in (see above) |
| `GET /api/me/orders` | the orders of the request's Telegram chat (initData), newest first, `{orders, counts}` — the Mini App list, no number+token pairs |
| `POST /api/orders` | order form state + `promoCode`; **requires a client session** (bearer or initData); supports `Idempotency-Key`; `503` when maintenance |
| `GET /api/orders/{number}` | status page; `?token=` or the owning chat identity returns the full order, otherwise the operator's private fields are hidden |
| `POST /api/orders/lookup` | `{number, token}` → `Order` (used by `/track`) |
| `POST /api/orders/batch` | `{items:[{number, token}]}` → `Order[]` (used by `/profile`) |
| `POST /api/orders/{number}/claim` | `{token?, senderName}` → `payment_claimed`; the Mini App omits the token — the chat identity authorizes the claim |
| `GET /api/orders/{number}/result` | signed download (`?sig=`, `?token=`, an operator session, or the owning chat) |
| `GET /api/reviews?status=published&limit=&offset=` | reviews, newest first |
| `POST /api/reviews` | `ReviewInput` → `pending` review |
| `GET /api/settings` | public settings incl. the Kaspi QR url and `paymentLink` (the «Оплатить через Kaspi» button) |
| `GET /api/settings/payment-qr` | the uploaded QR image (fallback when no payment link is set) |

### Admin (`Authorization: Bearer <token>`)

| Method & path | Notes |
|---|---|
| `POST /api/admin/auth/login` | `{operatorId, code}` (frontend) or `{operatorId, password, totp}`; `200 {session, token}`, `{step:"totp_required"}` when a second factor is configured |
| `POST /api/admin/auth/logout`, `GET /api/admin/me` | session lifecycle |
| `POST /api/admin/auth/telegram/start`, `.../poll` | Telegram login |
| `GET /api/admin/orders?status=&q=&sort=&page=&limit=&shape=` | `{orders, counts, total, page, limit}`; `?shape=array` for a plain `Order[]`; **`payment_claimed` first** |
| `GET /api/admin/orders/{number}` | full order with events, attachments, notes |
| `POST /api/admin/orders/{number}/confirm` | `payment_claimed → queued` |
| `POST /api/admin/orders/{number}/reject` | `{reason}` → `payment_rejected` |
| `POST /api/admin/orders/{number}/cancel` | `{reason?}` |
| `POST /api/admin/orders/{number}/retry` | `failed_final`/`payment_rejected` → `queued` |
| `PATCH /api/admin/orders/{number}` | `{operatorNote}` (saved on blur) |
| `GET /api/admin/reviews?status=` | `{reviews, counts}`; `?shape=array` for `Review[]` |
| `PATCH /api/admin/reviews/{id}` | `{status: published\|rejected\|pending}` |
| `GET/PUT /api/admin/settings` | settings incl. `maintenance`, `queuePaused` |
| `POST /api/admin/settings/payment-qr` | multipart (`file` or `qr`), ≤ 1.5 MB, magic-byte checked |
| `GET/PUT /api/admin/pricing` | validated: integer prices > 0, `min ≤ max` |
| `GET/POST /api/admin/promos`, `PATCH/DELETE /api/admin/promos/{id}` | duplicate codes → `Промокод X уже существует` |
| `GET /api/admin/stats` | counts, revenue, queue depth, average generation, failure rate, disk usage, oldest queued order (> 30 min → `queueWarning`) |

## 5. Business rules implemented server-side

- **Price** (`internal/pricing`): `subtotal = unit_price × units × works`;
  volume tier on `works` (10/15/20 % from 5/10/20 works); promo applies to the
  post-volume sum; all integers, `Math.round` equivalent. Orders store `amount` and
  `quoteLines` frozen at creation - editing prices never changes old orders.
- **Promo codes** (`internal/promo`): the validation order and the exact Russian
  messages of `checkPromo()` (disabled → expired → usage limit → first order →
  product → min units → min amount). A machine code is returned next to each
  reason. `usedCount` grows when the order is created; cancelling does not restore
  it. The order endpoint re-checks the code: if it no longer qualifies, the order is
  created without the discount and the response carries `promoDropReason`.
- **First order**: for a signed-in client the backend counts the orders already
  attached to the chat; without a session it falls back to the contact name -
  documented in `service.IsFirstOrder`.
- **Authorization** (`internal/service/auth_client.go`, `auth_admin.go`): every
  web session is opened through the Telegram bot - the site shows a one-time code,
  the chat confirms it with `/login <code>` and the poll exchanges it for a
  session. An operator chat (bound via `/bind` or listed in
  `TELEGRAM_ADMIN_CHAT_IDS`) also receives a panel session, so one login covers
  both roles. The password endpoint (`POST /api/admin/auth/login`) stays as a
  technical/break-glass path and is not part of the UI; 2FA was removed with the
  password form.
- **Order gate**: `POST /api/orders` requires a client session (`Authorization: Bearer`
  or the Mini App `X-Telegram-Init-Data` header, otherwise `401`). The order stores the
  session's `telegram_chat_id` and `@username`, so the client gets every status push without
  manual linking (`internal/service/orders.go`, `store.ClientSession`).
- **Chat identity in the Mini App** (`internal/service/orders.go`): `GET /api/me/orders`
  lists the chat's orders; the own order card, «Я оплатил» and the result download are
  authorized by the validated initData chat without any access token. A foreign chat gets
  the same neutral `404` as an unknown number, so ownership never leaks.
- **Operator panel inside the Mini App** (`internal/service/auth_admin.go`): `/api/admin/*`
  accepts the same initData header when the chat is bound to an operator (`/bind`,
  `TELEGRAM_ADMIN_CHAT_IDS` or the seeded chat id) — `AdminSessionFromInitData` opens the
  panel without a password or a login code, and `/api/auth/me` reports the role so the app
  can show the entry. The password endpoint stays as the break-glass path.
- **File delivery** (`internal/tgbot`, `internal/service/notify.go`): a finished document is
  queued with its storage path and sent by the bot loop with `sendDocument` (caption =
  the completion message, text fallback when the TTL elapsed).
- **Payment link** (`internal/domain`, migration `0007_payment_link.sql`): the operator's
  Kaspi QR deep link (`https://qr.kaspi.kz/…`) lives in the settings row and is what the
  Mini App puts behind the red «Оплатить через Kaspi» button; the uploaded QR image is the
  fallback. Only `http(s)` links are accepted (`service.SaveSettings`).
- **State machine** (`internal/store/transitions.go`): every transition is a
  transaction with `SELECT ... FOR UPDATE`, a status guard, an `order_events` row and
  a push notification. Illegal transitions answer `409 conflict`.
- **Validation** (`internal/validate`): mirrors `validate.ts` field by field,
  including `units` ranges per product, file limits/types and the `«Ануар А.»`
  sender-name rule.
- **Generation** (`internal/worker`): the confirmation puts an order into the
  queue; the worker runs up to `WORKER_MAX_AGENTS` (3) agents in parallel, one
  order per fresh working directory `<GENERATOR_DIR>/work/<number>/`:
  `.clinerules/01-skill.md` (the skill), `skill/` (its references, templates and
  scripts), `task.json` (the order brief), `agent.log` (the raw agent stream,
  written live) and the produced `result.docx` / `result.pptx`. The skill follows
  the product: **satbayev-gost** for essay/lab/other, **pptx** for presentations.
  The prompt asks for the document only: no report, and the agent removes its own
  temporary files before finishing (only the result and the service files stay).
  The file is validated (ZIP/OOXML signature), moved into the regular storage and
  the order flips to `completed`; failures are retried (`WORKER_AGENT_RETRIES`),
  then the order becomes `failed_final` with the agent log for the operator. A
  missing agent CLI falls back to the built-in `.docx` writer in `auto` mode
  (`GENERATOR_MODE`), and orders stuck in `generating` are requeued (the
  threshold never undercuts the configured timeout, so a live run is safe).
- **Admin console** (`GET /api/admin/orders/{number}/console`): the operator can
  watch a run live - the endpoint renders the agent stream into readable events
  (iterations, tool commands, token usage, errors, the final result) and the
  order page polls it while `generating`. The raw stream is downloadable via
  `.../console/log`.
- **Security** (README section 11): Argon2id hashes for the technical password,
  bot-only panel access with a 15-minute lockout after 5 failed password attempts,
  server-side sessions (only hashes stored), order tokens stored as peppered
  SHA-256, identical errors for a wrong order/token pair, rate limits (`429` +
  `Retry-After`) on order creation, client sign-in, reviews and promo checks,
  magic-byte checks on uploads, no internal details in responses.

## 6. Database

Migrations live in `internal/store/migrations` and run automatically at startup
(`schema_migrations`). Tables: `product_pricing`, `volume_discounts`, `orders`,
`order_events`, `order_files`, `order_jobs`, `reviews`, `promo_codes`,
`promo_code_products`, `promo_code_usages`, `app_settings`, `operators`,
`admin_sessions`, `login_attempts`, `telegram_login_challenges`, `counters`,
`idempotency_keys`, `app_kv`, `telegram_outbox`.

Demo data (`internal/seed`, port of `seed.ts`): 10 orders across the whole
lifecycle, 11 reviews (6 published / 4 pending / 1 rejected), 6 promo codes
(`SRUNAL10`, `FIRST15`, `WINTER20`, `LABHIT10`, `SPRING25`, `OLDFIVE`), prices,
settings and the demo operator **`@operator_almaty` / `Eshkere9113!xxx!`** on a
fresh database.

Change an operator password (or create a new operator) without touching the code:

```powershell
go run ./cmd/api -operator '@operator_almaty' -password 'NewSecret!2026'
go run ./cmd/api -operator '@second_operator' -password 'Secret!2026' -operator-name 'Оператор Алматы'
```

The command updates only the password hash; the password is the technical
break-glass path (the panel signs in through the bot), and it is visible in the
process list while the command runs - prefer rotating from a terminal you control.

**Documented deviations from the mock**

1. Demo access tokens: `seedToken()` in `seed.ts` mixed only two digits, so every
   seeded order got the same token (`CLS28G`); PostgreSQL enforces uniqueness, so
   the seed derives a unique token per order (`internal/seed/seedToken`). Orders
   created through the API always use cryptographically random tokens.
2. `GET /api/orders/{number}` hides `contact`, `clientNote`, `operatorNotes` and
   `senderName` (the mock returned everything to everyone). Add `?token=` or use the
   admin endpoint for the full order.
3. `GET /api/admin/orders` and `/api/admin/reviews` return `{orders|reviews, counts}`
   envelopes; `?shape=array` gives the plain array the current frontend expects.
4. Extra fields the frontend can ignore: `resultFile`, `promoDropReason`.

## 7. Configuration reference

Everything lives in `.env` (see `.env.example` for comments): `DATABASE_URL`,
`HTTP_ADDR`, `CORS_ORIGINS`, `PUBLIC_BASE_URL`, `FRONTEND_BASE_URL`, `TOKEN_PEPPER`,
`SESSION_TTL` (panel session, 12 h), `CLIENT_SESSION_TTL` (client session opened
through the bot, 30 days), `LOGIN_MAX_ATTEMPTS`, `LOGIN_LOCKOUT`, `STORAGE_DIR`,
`RESULT_TTL`, `SEED_ON_START`, `RATE_LIMIT_*`, `TRUST_PROXY`, `WORKER_*`,
`TELEGRAM_*`.

Generation (`WORKER_*`, `GENERATOR_*`, `SKILL_*`):

| Variable | Default | Meaning |
|---|---|---|
| `WORKER_MAX_AGENTS` | `3` | orders generated at the same time |
| `WORKER_AGENT_RETRIES` | `1` | extra attempts after a failed agent run |
| `GENERATOR_MODE` | `auto` | `agent` (always the CLI), `auto` (CLI or built-in), `builtin` |
| `GENERATOR_COMMAND` | `cline` | the agent CLI (Cline) |
| `GENERATOR_PROVIDER` / `GENERATOR_MODEL` | `opencode-go` / `deepseek-v4.1-flash` | pinned model of every run (`-P` / `-m`) |
| `GENERATOR_THINKING` | `high` | reasoning effort: `none\|low\|medium\|high\|xhigh` |
| `GENERATOR_API_KEY` | – | optional BYOK key, exported under the provider's own variable (`opencode-go` → `OPENCODE_API_KEY`, `deepseek` → `DEEPSEEK_API_KEY`, …) |
| `GENERATOR_API_KEY_ENV` | – | override that variable name (comma-separated) |
| `GENERATOR_CONFIG_DIR` / `GENERATOR_DATA_DIR` | – | where the CLI keeps its config/state (Docker: `/data/cline`) |
| `GENERATOR_TIMEOUT` | `20m` | per-order budget (presentations may need more) |
| `GENERATOR_DIR` | `var/generator` | per-order working directories |
| `SKILL_SOURCE_DIR` | `~/.agents/skills` | global skills root (Docker: `/app/skills`) |
| `GENERATOR_SKILL_GOST` / `GENERATOR_SKILL_PPTX` | `satbayev-gost` / `pptx` | skill per product |

`TOKEN_PEPPER` protects order tokens, session tokens and result links - generate one
with `openssl rand -hex 32` before production. Changing it invalidates existing
tokens.

## 8. Known gaps (next steps)

- Payments stay manual: there is no Kaspi API integration (check the sender name and
  the amount, then confirm in the panel or via the bot).
- Cloudflare Turnstile is not wired in (the frontend does not send a token yet);
  rate limits and the promo/order limits are in place.
- Client material is stored as metadata (`attachments`); the multipart upload
  endpoint for real files is the next step (the schema already supports
  `order_files` with a storage key).
- The generation worker produces a formatted brief, not the finished academic
  document: plug the real generator into `worker.finish` (the queue, jobs, retries
  and notifications are ready).
- Metrics are exposed through `GET /api/admin/stats`; OpenTelemetry/Prometheus
  exporters are not configured yet.
