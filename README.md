# SruNal KZ — Telegram bot + Mini App for academic work

**English** · [Русский](README-ru.md)

> **The service is live: [@srunal_bot](https://t.me/srunal_bot)** - open the Mini App
> from the bot's menu button.

A Telegram-first service for academic work: one bot takes the orders and pushes
the notifications, while the Mini App walks the client through the flow (quote →
Kaspi QR payment → operator verification → the finished document in the chat). The
business logic (pricing, promos, the generation queue) lives in the Go backend; the
Mini App is the client surface served by that same API (one origin, no CORS,
`initData`-only authentication).

## The flow

1. **Order.** The client taps "Order" and walks a 4-step wizard: work type and volume →
   topics and materials → title-page data → confirmation. The price is computed by the
   server (`POST /api/quote`); volume discounts and promo codes are applied server-side.
2. **Payment.** The order screen shows a red "Pay with Kaspi" button (a `qr.kaspi.kz`
   deep link) and the scannable QR code. The client marks "I have paid" (with the Kaspi
   sender name), the operator checks the statement and confirms the payment.
3. **Generation.** An AI agent (Cline) builds the document with the `satbayev-gost`
   skill (or `pptx`): Times New Roman 14, 1.5 spacing, 30/15/20/20 mm margins, a title
   page in the Satbayev University standard, references per GOST 7.1-2003. Several
   ordered works become several separate documents (one per topic).
4. **Delivery.** The finished file arrives in the bot chat and stays downloadable in the
   Mini App for 30 days.
5. **Rework.** The client writes what has to be fixed, the operator confirms and sends
   the order back to the agent - the agent applies the corrections to the same file.

## Operator panel (inside the Mini App, `/operator`)

- Order registry with filters and search, receipt check (confirm/reject with a reason).
- Live generation console: iterations, agent commands, tokens and cost; the raw log opens
  in any browser through a signed link (no password, no Telegram session).
- Generation spend in USD and KZT (the USD→KZT rate comes from the National Bank of
  Kazakhstan and refreshes itself), net revenue in the statistics.
- Send a finished work back for rework with the client's corrections (and the operator's).
- Reviews moderation, promo codes, pricing, service settings (QR, queue pause).
- Several operators: `/bind` in the bot links a chat to an account without unlinking
  anyone else.

## Interface

- Languages: Russian, Kazakh, English (Profile → "Interface language").
- Theme: as in Telegram / light / dark (Profile → "Theme").
- The order draft survives an app restart; title-page templates are reusable.

## Repository layout

```
├── backend/            Go 1.27: API + Telegram bot + generation worker (one process)
│   ├── cmd/api           # HTTP API, bot and worker
│   ├── cmd/bot           # bot and worker only
│   ├── cmd/agentcheck    # probe the agent CLI
│   ├── internal/         # api · auth (initData) · service · store · tgbot · worker
│   ├── skills/           # pptx (published) · satbayev-gost (not published, see skills/README.md)
│   └── scripts/          # setup_db.sql · smoke.ps1
├── miniapp/            React + Vite + TypeScript + Tailwind Mini App
│   ├── src/telegram/      # SDK: initData, theme, safe areas, MainButton/BackButton
│   ├── src/i18n/          # ru / kk / en
│   ├── src/api/           # the single API client (initData header)
│   ├── src/screens/       # Home · wizard · order · my orders · reviews · support · profile · operator
│   └── scripts/           # dev-initdata.mjs · smoke-api.mjs · e2e-live.mjs
├── design/stitch/      design-system screens and specification (reference)
├── deploy/             Dockerfile (backend + built Mini App in one image), compose, TLS, runbooks
├── MIGRATION_PLAN.md   the Telegram-first migration plan
├── STACK.md            how the system works end to end
├── README.md           this document (English)
└── README-ru.md        Russian version
```

## Stack

Go 1.27 · PostgreSQL 17 · React 18 + Vite + TypeScript + Tailwind CSS 4 · Caddy
(auto-HTTPS) · Docker Compose · Telegram Bot API (long polling) · Telegram Mini App SDK.

## Quick start (development)

```powershell
# 1. Database (once) - details in backend/README.md
$env:PGPASSWORD="<postgres-password>"
psql -h 127.0.0.1 -U postgres -f backend/scripts/setup_db.sql
psql -h 127.0.0.1 -U postgres -c "create database srunal owner srunal"

# 2. Backend
cd backend
Copy-Item .env.example .env        # DATABASE_URL, TOKEN_PEPPER, bot token
go run ./cmd/api                   # :8080 (migrations + seed on start)

# 3. Mini App
cd ../miniapp
npm install
npm run dev                        # :5174, /api proxied to :8080
npm run build                      # tsc --noEmit + vite build -> dist/
```

Outside Telegram the app shows the gate screen by design - `initData` only exists
inside the messenger. For local development `miniapp/scripts/dev-initdata.mjs` mints a
correctly signed payload:

```powershell
cd miniapp
node scripts/dev-initdata.mjs            # prints auth_date=…&user=…&hash=…
# in the browser console:
#   localStorage.setItem('srunal.dev.initData', '<printed string>')
#   location.reload()
```

## Deployment

```powershell
cd deploy
.\push.ps1 -Server root@<host> -Domain <domain> -MiniAppShortName <short-name> -SyncAgentCredentials
```

The script packs the sources, uploads them and rebuilds the containers (`db` + `api` +
`caddy` with auto-HTTPS); migrations are applied when the API starts. The full runbook
for a fresh VDS is in `deploy/VDS.md`, the ssh template in `deploy/SSH.md`.

## Security & privacy

No secrets and no client personal data are committed:

- `.env` (bot token, `TOKEN_PEPPER`, database password), `deploy/.env` and the model
  provider credentials are gitignored; only `.env.example` files with empty values are
  in the repository.
- `backend/var/` (orders, documents, agent logs, work directories) is gitignored as well.
- Client authentication is signed-`initData` only; order and session tokens are stored
  hashed with a pepper, operator passwords are hashed with Argon2id; the operator panel
  opens only for a bound chat.
- Personal data (names, topics, documents, chats) lives in the database and the server
  storage only - never in the code or the logs.
- The `satbayev-gost` skill (official title-page template and university logo) is
  **not published**: it stays in your working copy and in your deployment, while the
  repository only carries `backend/skills/README.md` with the installation notes.
- The deployment host, domain and credentials are intentionally not published.

## Documentation

| Document | What it covers |
|---|---|
| `STACK.md` | the whole system: backend, worker, bot, database, deploy |
| `MIGRATION_PLAN.md` | the Telegram-first migration principles and plan |
| `backend/README.md` | backend reference: API, bot commands, generation, configuration |
| `miniapp/README.md` | Mini App: screens, i18n, theme, local development |
| `deploy/VDS.md` | runbook: fresh install, operators, agent, recovery |
| `deploy/SSH.md` | ssh template and the usual server operations |
| `design/stitch/` | design-system screens and tokens |

## Tests

```powershell
cd backend
go test ./internal/...                                   # unit tests
$env:TEST_DATABASE_URL="postgres://srunal:srunal@127.0.0.1:5432/srunal_test?sslmode=disable"
go test ./internal/store/...                             # database integration tests
go run ./cmd/agentcheck                                  # probe the agent CLI
cd ../miniapp
npm run build                                            # tsc --noEmit + vite build
node scripts/smoke-api.mjs                               # HTTP smoke of the Mini App flow
```

## License

**MIT** (see `LICENSE`, Copyright © 2026 SruNal KZ): use, modify and redistribute with
the copyright notice preserved. Satbayev University materials (the official title-page
template, the logo) are not covered by the license and are not published in this
repository.
