# SruNal KZ — Full Stack & Architecture Guide

> **Status:** v1.0 · 17 October 2026
> **Scope:** How the SruNal KZ system is built and how it works end to end — the client web app,
> the Go backend with the generation worker, the Telegram bot, the database, and the deployment.
> **Audience:** Developers joining the project, reviewers, and anyone preparing the Telegram-first
> rebuild in this folder (`srunal_tgbot_miniapp`).
> **Note:** This document describes the **currently implemented system** (website + bot).
> The planned Telegram-first direction is summarised in section 18 and specified in detail in
> `../srunal_vite/design_md/design.md` and `../srunal_vite/design_md/prompt.md`.

---

## Table of contents

1. What the system is
2. System overview
3. Repository layout
4. Technology stack
5. Runtime architecture
6. End-to-end order lifecycle
7. Domain rules
8. Backend internals
9. HTTP API
10. Telegram bot
11. Generation pipeline
12. Frontend application
13. Security model
14. Configuration reference
15. Running & testing
16. Deployment topology
17. Known gaps & documented deviations
18. The Telegram-first target
19. Glossary

---

## 1. What the system is

SruNal KZ is a paid academic-document service for **Satbayev University (КазНИТУ им. К.И. Сатпаева,
Almaty)**. Students order a document — an essay (реферат), a lab report (лабораторная), a
presentation (презентация), or another assignment (СРС/СРСП) — pay via **Kaspi QR**, an operator
verifies the payment manually, and an **AI generation pipeline** produces a formatted `.docx` or
`.pptx` that the client downloads. Support contact: `@meownuarka`.

| Actor | Surface | What they do |
|---|---|---|
| Client (student) | Web app (today) / bot chat | Places orders, pays, tracks status, downloads the file, leaves reviews |
| Operator | Admin panel + Telegram bot | Verifies payments, confirms/rejects, retries generation, manages promos/pricing/settings |
| System | Worker + bot | Runs the generation queue, sends every notification, expires stale orders |

Key business constraints that shape the whole stack:

- **Payments are manual** (no Kaspi API integration): a QR image + sender name + amount + time window, checked by a human.
- **Prices are computed server-side only**; clients never send a price.
- **Every state change is announced** in the client's Telegram chat.
- **Result files live 30 days** (`RESULT_TTL=720h`) and are delivered as a signed link and as a Telegram document.
- Display time zone is **Asia/Almaty**; all API timestamps are ISO 8601 UTC; money is integer KZT (`₸`).

---

## 2. System overview

```
                        ┌─────────────────────────────────────────────┐
                        │                 Clients                     │
                        │   Browser (React SPA)  ·  Telegram chat     │
                        └───────────────┬───────────────┬─────────────┘
                                        │ HTTPS/JSON    │ long polling (bot)
                                        ▼               ▼
        ┌────────────────────────────────────────────────────────────────────┐
        │                     srunal-api  (single Go process)                │
        │                                                                    │
        │  ┌──────────────┐  ┌───────────────┐  ┌──────────────┐  ┌────────┐ │
        │  │  HTTP API    │  │  Service core │  │  TG bot      │  │ Worker │ │
        │  │  /api/*      │→ │  (business    │ ←│  long poll   │  │ queue  │ │
        │  │  net/http    │  │   rules)      │  │  + outbox    │  │ +agent │ │
        │  └──────────────┘  └───────┬───────┘  └──────┬───────┘  └───┬────┘ │
        │                            │                 │              │      │
        └────────────────────────────┼─────────────────┼──────────────┼──────┘
                                     ▼                 ▼              ▼
                            ┌─────────────┐   Telegram Bot API   ┌─────────────┐
                            │ PostgreSQL  │   (api.telegram.org) │  Storage    │
                            │   17        │                      │  /data      │
                            │ orders, DB  │                      │ files, work │
                            └─────────────┘                      │ dirs, cline │
                                                                 └─────┬───────┘
                                                                       ▼
                                                              ┌─────────────────┐
                                                              │ Cline CLI agent │
                                                              │ + skills        │
                                                              │ (satbayev-gost, │
                                                              │  pptx)          │
                                                              └─────────────────┘
```

The API, the generation worker, and the Telegram bot all run **inside one process** (`cmd/api`).
`cmd/bot` exists for a split deployment (bot + worker without HTTP). There is no message queue
infrastructure: the PostgreSQL database is the queue (status + `order_jobs` + `SELECT ... FOR UPDATE`).

---

## 3. Repository layout

```
C:\path\to\projects\
├── srunal_go\                  # Go backend: API + worker + bot (this doc's focus)
├── srunal_vite\                # React SPA: landing, order flow, tracking, admin panel
│   └── design_md\
│       ├── design.md           # Design spec of the future Telegram Mini App
│       └── prompt.md           # Google Stitch prompt kit for the Mini App screens
└── srunal_tgbot_miniapp\       # This folder — the Telegram-first rebuild lives here
    └── STACK.md                # This document
```

### 3.1 `srunal_go` (backend)

```
srunal_go/
├── cmd/
│   ├── api/            # main entrypoint: HTTP API + worker + bot in one process
│   └── bot/            # bot + worker only (when the API runs elsewhere)
├── internal/
│   ├── api/            # routes, JSON envelope, CORS, rate limiting, admin session guard
│   ├── auth/           # Argon2id hashing, session tokens, (TOTP helpers)
│   ├── catalog/        # products, units, institutes, statuses (mirror of catalog.ts)
│   ├── config/         # env + .env loader and validation
│   ├── domain/         # API types (mirror of types.ts): Order, Review, PromoCode, Settings…
│   ├── format/         # money / plural rendering (mirror of format.ts)
│   ├── pricing/        # price formula + volume tiers (mirror of pricing.ts)
│   ├── promo/          # promo validation order and exact Russian messages
│   ├── seed/           # demo dataset (orders, reviews, promos, pricing, settings, operator)
│   ├── service/        # business logic shared by API, worker and bot
│   ├── store/          # PostgreSQL access + migrations/*.sql (auto-applied at startup)
│   │   └── migrations/ # 0001_init … 0005_order_work_kind
│   ├── tgbot/          # Telegram bot: raw Bot API client, long polling, commands
│   ├── validate/       # form validation (mirror of validate.ts)
│   └── worker/         # generation queue, agent runner, built-in .docx writer, skills
├── skills/
│   ├── satbayev-gost/  # SKILL.md + references for essay/lab/other documents
│   └── pptx/           # SKILL.md for presentations
├── scripts/
│   ├── setup_db.sql    # creates the PostgreSQL role + database
│   └── smoke.ps1       # end-to-end HTTP smoke test (76 checks)
├── var/                # runtime storage: storage/, generator/work/<order>/, …
├── Dockerfile
├── docker-compose.yml
└── .env.example        # every setting, documented
```

### 3.2 `srunal_vite` (frontend)

```
srunal_vite/
├── src/
│   ├── app/router.tsx        # createBrowserRouter data router, lazy pages
│   ├── components/           # ui.tsx, form.tsx, layout.tsx, KaspiCard, GenerationConsole,
│   │                         # ReviewModal, ThemeToggle, ToastProvider, route progress…
│   ├── context/toast-context.ts
│   ├── lib/                  # api.ts (the single integration seam), session.ts, types.ts,
│   │                         # catalog.ts, format.ts, validate.ts, pricing.ts, storage.ts,
│   │                         # legal.ts, env.ts, theme.ts, hooks.ts, clipboard.ts, seed.ts
│   ├── pages/                # Home, OrderForm, OrderStatus, Track, Reviews, Profile, Legal,
│   │   └── admin/            # Login + admin: Orders, OrderDetail, Reviews, Promos, Stats,
│   │                         # Pricing, Settings
│   └── styles/               # tokens.css (light+dark), base.css, components.css, pages.css
├── public/                   # favicon, demo assets
├── index.html                # pre-paint theme script, Inter + Material Symbols fonts
└── vite.config.ts
```

---

## 4. Technology stack

### 4.1 Frontend

| Layer | Choice | Version (package.json) |
|---|---|---|
| UI library | React | `^19.2.8` |
| DOM runtime | react-dom | `^19.2.8` |
| Routing | react-router-dom (data router, lazy routes) | `^7.18.4` |
| Language | TypeScript | `~6.0.2` |
| Build tool | Vite | `^8.3.0` (`@vitejs/plugin-react ^6.1.1`) |
| Lint | ESLint + typescript-eslint + react-hooks + react-refresh | `^10.10.0` / `^8.69.0` / `^7.1.1` / `^0.5.6` |
| Styling | Plain CSS with custom properties (design tokens), no UI framework | — |
| Fonts / icons | Inter (400–700) + Material Symbols Outlined, loaded from Google Fonts | — |
| Themes | `data-theme="light|dark"` on `<html>`, pre-paint inline script, View Transitions cross-fade | — |
| State | Local component state + small custom hooks (`useAsync`, `useDebounced`); no Redux/Zustand | — |

The frontend talks to the backend through exactly one module, `src/lib/api.ts`; pages never call
`fetch` directly. `ApiError` carries the server envelope (`code`, `status`, `fields`, `retryAfter`).

### 4.2 Backend

| Layer | Choice | Version |
|---|---|---|
| Language | Go | `1.27.2` (module `github.com/srunal/srunal-go`) |
| HTTP | Standard library `net/http` — no web framework | — |
| Database driver | `github.com/jackc/pgx/v5` (direct SQL, no ORM) | `v5.11.0` |
| Cryptography | `golang.org/x/crypto` — Argon2id password hashing, HMAC, token hashing | `v0.58.0` |
| Logging | Standard library `log/slog` (text handler) | — |
| Telegram | Hand-rolled Bot API client (long polling) — no third-party bot library | — |
| Migrations | Plain `.sql` files embedded in the binary, auto-applied at startup | 5 files |

The dependency list is intentionally tiny: pgx + x/crypto. Everything else — router, rate limiter,
session store, Telegram client, document writer — is first-party code under `internal/`.

### 4.3 Data & files

| Layer | Choice |
|---|---|
| Database | PostgreSQL **17** (`postgres:17-alpine` in Compose) |
| Object/file storage | Local filesystem under `STORAGE_DIR` (Docker volume `app-data` → `/data/storage`) |
| Result files | Signed URL (`?sig=`) served by the API; also sent as a Telegram document |
| Temporary artifacts | `GENERATOR_DIR/Work/<order-number>/` per-run working directories |

### 4.4 Telegram bot

| Layer | Choice |
|---|---|
| Transport | Long polling (`getUpdates`), no webhook, no public URL/TLS needed |
| Library | First-party client in `internal/tgbot/api.go` |
| Delivery guarantee | `telegram_outbox` table — messages persist when the bot token is absent/offline |
| Inline actions | `callback_query` buttons (operator confirm/reject), message rewritten after use |
| Auth | Bot-mediated login challenges (`telegram_login_challenges`) for both roles |

### 4.5 Document generation

| Layer | Choice |
|---|---|
| Agent | **Cline CLI** (`cline@latest`, Node 22) spawned per order |
| Model | Pinned per run: provider `opencode-go`, model `deepseek-v4.1-flash`, thinking `high` (override via env) |
| BYOK | Optional key `GENERATOR_API_KEY`, exported under the provider's env var (`opencode-go → OPENCODE_API_KEY`) |
| Skills | Global skills layout `<dir>/<skill>/SKILL.md`: `satbayev-gost` (essay/lab/other), `pptx` (presentations) |
| Document toolchain | Python 3 + `python-docx`, `python-pptx`, `lxml`; `pandoc` for conversions |
| Fallback | Built-in Go `.docx` writer (`GENERATOR_MODE=builtin|auto`) |

### 4.6 Infrastructure & tooling

| Layer | Choice |
|---|---|
| Container build | Multi-stage `Dockerfile`: `golang:1.27-bookworm` → `node:22-bookworm-slim` |
| Orchestration | `docker compose` — services `db` (postgres:17-alpine) + `api` |
| Volumes | `db-data` (PostgreSQL), `app-data` (storage, generator work dirs, Cline config) |
| Health check | `curl /api/health` inside the container; compose waits for `pg_isready` |
| Tests | `go test ./internal/...` (unit) + `go test ./internal/store/...` (DB integration, needs `TEST_DATABASE_URL`) + `scripts/smoke.ps1` (76 end-to-end HTTP checks) |
| Frontend build | `tsc -b && vite build`, `eslint .` |
| Time zone | `TZ=Asia/Almaty` in the container |

---

## 5. Runtime architecture

### 5.1 One process, three loops

`cmd/api/main.go` boots in this order:

1. Parse flags — `-migrate` (migrations only), `-seed` (force demo seed), `-operator @handle -password ...` (rotate an operator password and exit).
2. `config.Load()` — env + `.env`.
3. `store.Open()` — pgx pool; `store.Migrate()` applies `internal/store/migrations/*.sql` (tracked in `schema_migrations`).
4. `service.New()` — the business core shared by all surfaces; `EnsureStorage()` creates directories.
5. `seedIfNeeded()` — imports the demo dataset into an empty database when `SEED_ON_START=true`.
6. `api.New()` + `http.Server` (ReadHeader 10 s, Read 30 s, Write 60 s, Idle 90 s).
7. Background goroutines:
   - **worker** — the generation queue (unless `WORKER_ENABLED=false`; can start paused via `WORKER_QUEUE_PAUSED_ON_START`);
   - **housekeeping loop** — expires stale orders/sessions, requeues orders stuck in `generating`, cleans result files past `RESULT_TTL`;
   - **tgbot** — long polling when `cfg.BotConfigured()`.
8. Graceful shutdown on `SIGINT`/`SIGTERM` via `signal.NotifyContext`.

`cmd/bot` runs steps 1–5 + worker + bot only — for deployments where the HTTP API is hosted
separately. Both binaries build from the same `internal/` code.

### 5.2 Process boundaries

- No message broker: **PostgreSQL is the queue** (status transitions guarded with `SELECT ... FOR UPDATE`, `order_jobs` for attempts, `idempotency_keys` for duplicate submits).
- No cron: every periodic task is a goroutine ticker inside the process.
- No external CDN: the SPA is served separately (dev: Vite; prod: any static host / reverse proxy).

---

## 6. End-to-end order lifecycle

This is the spine of the whole system; every component exists to move one order through it.

```
 Client                API/Service            Operator         Worker          Bot
   │                       │                     │                │              │
   │ 1 POST /api/quote     │                     │                │              │
   │──────────────────────▶│ price formula       │                │              │
   │                       │                     │                │              │
   │ 2 POST /api/orders    │                     │                │              │
   │  (client session)     │ number DG-xxxxxx    │                │              │
   │──────────────────────▶│ status:             │                │              │
   │                       │ awaiting_payment    │                │              │
   │◀──────────────────────│ order + token       │                │              │
   │                       │                     │                │              │
   │ 3 Kaspi QR payment    │                     │                │              │
   │   4 POST …/claim      │                     │                │              │
   │──────────────────────▶│ payment_claimed ────┼───────────────▶│              │
   │                       │                     │ 5 confirm      │              │
   │                       │◀────────────────────┼───────────────│              │
   │                       │ status: queued ─────┼───────────────┼─────────────▶│ «Оплата
   │                       │                     │                │              │  подтверждена»
   │                       │                     │ 6 worker picks │              │
   │                       │                     │    up → runs agent (Cline)   │
   │                       │                     │                │ workdir:     │
   │                       │                     │                │ .clinerules/ │
   │                       │                     │                │ task.json    │
   │                       │                     │                │ result.docx  │
   │                       │ status: completed ◀─┼────────────────│              │
   │◀──────────────────────┼─────────────────────┼────────────────┼──────────────│ «Работа
   │ 7 GET …/result?sig=   │                     │                │              │  готова»
   │──────────────────────▶│ signed file         │                │              │
   │                       │                     │                │              │
   │ 8 POST /api/reviews   │ pending review ─────┼─▶ moderation ──┼──────────────│
```

### 6.1 Quote and create

- The form builds the order state; **every price is recomputed server-side**.
- `POST /api/quote` — `{product, works, units, promo?}` → `Quote` with both numbers and
  human-readable `lines` (base, volume discount, promo, total). Debounced 300 ms in the UI; p95 < 1 ms.
- `POST /api/orders` — requires a **client session** (`Authorization: Bearer <client token>`,
  otherwise `401`); supports an `Idempotency-Key` header so a double submit returns the same order;
  returns the full `Order` including its access `token`. The order stores `telegram_chat_id` and
  `@username` from the session, so the bot can notify without any linking step.
- The order number (`DG-260391`) comes from a `counters` table; the access token is 6 characters
  from an unambiguous alphabet (`ABCDEFGHJKLMNPQRSTUVWXYZ23456789`), stored **hashed with `TOKEN_PEPPER`**.

### 6.2 Payment (Kaspi QR, manual verification)

1. The client opens `/order/:number` and scans the Kaspi QR from `GET /api/settings/payment-qr`
   (recipient name is printed on the card).
2. The client types **the sender name exactly as in Kaspi** (`Ануар А.` — ≥ 2 words, last word ≤ 2 chars)
   and presses «Я оплатил» → `POST /api/orders/{number}/claim` → status `payment_claimed`.
3. The operator receives a Telegram message with amount + sender name and the buttons
   **✅ Подтвердить / ❌ Отклонить**; the same actions exist in the admin panel (with a
   "Я проверил(а) поступление" checkbox) and as bot commands.
4. There is **no payment code** any more — identification is by amount + sender name + time window.
   Rejections carry a reason the client sees, and the client can claim again.

### 6.3 Operator confirmation

- `confirm` → `queued` (worker picks it up next poll; respect `settings.queue_paused`).
- `reject` → `payment_rejected` (reason required, ≤ 300 chars).
- `cancel` → `cancelled`; `retry` → `queued` (new attempt, used for `failed_final`/`payment_rejected`).
- Every action writes an `order_events` row (actor `operator`) — the audit trail visible to the
  client as the order timeline and to the operator in the admin panel.

### 6.4 Generation

Described in detail in section 11. In short: the worker claims `queued` orders, builds a per-order
working directory with the order brief and the matching skill, runs the Cline agent (pinned model),
validates the produced OOXML file, moves it into storage, and flips the order to `completed`.

### 6.5 Delivery and review

- On completion the bot sends the document into the client's chat (`sendDocument`) plus buttons
  («Оставить отзыв», «Открыть заказ»), and the order page shows the signed download link.
- `GET /api/orders/{number}/result?sig=` serves the file for 30 days.
- The client submits a review (`POST /api/reviews`) tied to an existing order; it starts as
  `pending` and is published or rejected by the operator in the moderation screen.

---

## 7. Domain rules

### 7.1 Products and pricing

| `code` | RU name | Unit | Range | Price |
|---|---|---|---|---|
| `essay` | Реферат | pages (страницы) | 4–40 | 80 ₸ / page |
| `lab` | Лабораторная | work (fixed) | 1 | 1 000 ₸ flat |
| `presentation` | Презентация | slides (слайды) | 5–30 | 240 ₸ / slide |
| `other` | Другой док. (СРС, СРСП…) | work (fixed) | 1 | 1 000 ₸ flat |

Volume discount by **works in one order** (only the highest tier applies):

| Works | Discount |
|---|---|
| ≥ 5 | 10 % |
| ≥ 10 | 15 % |
| ≥ 20 | 20 % |

**Price formula (server-side; the client never sends a price):**

```
subtotal        = unit_price × units × works        // units = 1 for fixed products
after_volume    = subtotal - round(subtotal × tier_pct / 100)
promo_discount  = round(after_volume × promo_pct / 100)
total           = after_volume - promo_discount     // integers, KZT
```

Orders **store `amount` and `quote_lines` frozen at creation** — editing prices never changes
existing orders. The admin pricing screen shows a warning about this.

### 7.2 Promo codes

Validation order and the exact Russian messages (the server returns a machine code next to each reason):

| # | Check | Message |
|---|---|---|
| 1 | empty input | `Введите промокод` |
| 2 | unknown code | `Промокод не найден` |
| 3 | `active = false` | `Промокод отключён` |
| 4 | expired (`expiresAt < now`) | `Срок действия промокода истёк` |
| 5 | usage limit reached | `Лимит использования промокода исчерпан` |
| 6 | first-order-only and the client already has orders | `Промокод действует только на первый заказ` |
| 7 | product not in the promo's list | `Промокод действует только на «Реферат»` |
| 8 | units below `minUnits` | `Промокод действует от 5 страниц` |
| 9 | subtotal below `minAmount` | `Промокод действует от 1 000 ₸` |

- Checked live in the form (`POST /api/promos/check`) **and re-checked when the order is created**;
  if it no longer qualifies, the order is created without the discount and the response carries
  `promoDropReason`.
- `usedCount` grows when the order is created; cancelling does **not** restore it.
- **"First order"** for a signed-in client = the count of orders already attached to that chat
  (documented in `service.IsFirstOrder`); without a session it falls back to the contact name.
- Demo seeds: `SRUNAL10`, `FIRST15`, `WINTER20`, `LABHIT10`, `SPRING25` (expired), `OLDFIVE` (disabled).

### 7.3 Order statuses and state machine

| Status | RU label | Meaning |
|---|---|---|
| `awaiting_payment` | Ожидает оплату | Created; QR shown |
| `payment_claimed` | Оплата проверяется | Client pressed «Я оплатил» |
| `payment_rejected` | Оплата отклонена | Operator found no matching payment (reason attached) |
| `queued` | В очереди | Payment confirmed; waiting for the worker |
| `generating` | Генерируется | Agent run in progress |
| `completed` | Готово | Result file stored and delivered |
| `failed_final` | Проблема | Generation failed after retries |
| `expired` | Истёк | Payment window elapsed |
| `cancelled` | Отменён | Cancelled by the operator |

```
awaiting_payment ──claim──▶ payment_claimed ──confirm──▶ queued ──worker──▶ generating
      │  ▲                        │    ▲                    │                    │
      │  │                        │    │                    │                    ├─ok──▶ completed
      │  │                        │  retry                  │                    └─fail─▶ failed_final
      │  │                   reject│    │                    │                              │
      │  │                        ▼    │                    │                          retry│
      │  └────────claim──────── payment_rejected ────────────┘                              │
      │                                                                                     │
      ├─expire──▶ expired                       cancel (from awaiting/claimed/rejected/queued) ─▶ cancelled
      └────────────────────────────────────────────────────────────────────────────────────────┘
```

- Every transition is a **transaction** with `SELECT ... FOR UPDATE`, a status guard, an
  `order_events` row and a push notification. Illegal transitions answer `409 conflict`.
- `payment_claimed` orders are **always pinned to the top** of the admin registry — the
  operator's main working queue.

### 7.4 Identifiers and formats

| Item | Format | Notes |
|---|---|---|
| Order number | `DG-` + counter (`DG-260391`) | human-typable, shown in mono |
| Access token | 6 chars `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` | stored peppered-hashed (`token_hash`) |
| Session tokens | random, stored hashed | operator 12 h, client 30 days |
| Login code | 6 digits, single-use, short TTL | confirmed in the chat |
| Money | integer KZT, rendered `1 000 ₸` | non-breaking spaces |
| Times | ISO 8601 UTC in API | displayed in Asia/Almaty |

### 7.5 Validation (server mirrors the client field by field)

| Field | Rule | Message (RU) |
|---|---|---|
| `topics` | 3–5000 chars | `Укажите тему работы (минимум 3 символа)` |
| `units` | within the product range | `Допустимо от 4 до 40 страниц` |
| `attachments` | ≤ 3 files, ≤ 5 MB, PDF/DOCX/TXT/PNG/JPG | `Не больше 3 файлов` / `Файл больше 5 МБ` / `Этот формат не поддерживается` |
| academic fields | required, ≤ 200 chars | `Укажите кафедру`, `Укажите институт`, … |
| `senderName` (claim) | ≥ 2 words, last word ≤ 2 chars | `Укажите как в Kaspi: имя и фамилия одной буквой, например «Ануар А.»` |
| `consent` | must be checked | `Нужно согласие, чтобы создать заказ` |

The order form merges server `error.fields` into per-input errors and focuses the first invalid field.

---

## 8. Backend internals

### 8.1 Package map

| Package | Responsibility |
|---|---|
| `internal/api` | `server.go` (mux + middleware chain), `handlers_public.go`, `handlers_client_auth.go`, `handlers_admin.go`, `limiter.go` (per-IP rate limiting) |
| `internal/service` | Business core: `orders.go`, `quote.go`, `promos.go`, `reviews.go`, `settings.go`, `auth_client.go`, `auth_admin.go`, `notify.go` (outbox pushes), `files.go`, `console.go` (live agent stream for the panel) |
| `internal/store` | SQL access: `orders.go`, `transitions.go` (the state machine), `telegram.go` (outbox, challenges, bindings), `client_sessions.go`, `promos.go`, `reviews.go`, `pricing.go`, `operators.go`, `stats.go`, `store.go` (pool + migrations) |
| `internal/tgbot` | `api.go` (raw Bot API calls: getUpdates, sendMessage, sendDocument…), `bot.go` (poll loop, router), `commands.go` (command handlers) |
| `internal/worker` | `worker.go` (queue loop), `generator.go` (mode dispatch), `agent.go` (Cline process, workdir, prompt), `docx.go` (built-in writer), `skill/` (skill file plumbing) |
| `internal/auth` | Argon2id hashing, token generation/hashing, TOTP helpers |
| `internal/pricing` / `internal/promo` / `internal/validate` / `internal/format` / `internal/catalog` / `internal/domain` | Pure rule modules that mirror the frontend equivalents 1:1 |
| `internal/seed` | Demo dataset (10 orders, 11 reviews, 6 promos, pricing, settings, demo operator) |
| `internal/config` | Environment loading + defaults |

### 8.2 Database

Migrations (`internal/store/migrations/`), applied automatically at startup and tracked in
`schema_migrations`:

| File | Adds |
|---|---|
| `0001_init.sql` | All core tables (`orders`, `order_events`, `order_files`, `order_jobs`, `reviews`, `promo_codes`, `promo_code_products`, `promo_code_usages`, `product_pricing`, `volume_discounts`, `app_settings`, `operators`, `admin_sessions`, `login_attempts`, `telegram_login_challenges`, `counters`, `idempotency_keys`, `app_kv`, `telegram_outbox`) + indexes |
| `0002_drop_review_grade.sql` | Removes the legacy review grade column |
| `0003_client_auth.sql` | Client sessions opened through the bot |
| `0004_outbox_buttons.sql` | Inline-button payloads for the Telegram outbox |
| `0005_order_work_kind.sql` | "Work kind" metadata on orders (presentation vs document) |

Notable design points:

- `orders.token_hash` is unique and peppered (`TOKEN_PEPPER`); changing the pepper invalidates
  all tokens, sessions and result links (documented trade-off).
- Partial index `orders_claimed_idx … where status = 'payment_claimed'` powers the pinned operator queue.
- `order_events` is append-only; the client timeline and the admin audit both read it.
- `telegram_outbox` decouples notifications from bot availability: with an empty token the API
  works normally and the messages flush when the bot starts.

### 8.3 Conventions

- Base path `/api`, JSON only, `camelCase` field names exactly as in `src/lib/types.ts`.
- Error envelope:

```json
{ "error": { "code": "validation_failed", "message": "Проверьте выделенные поля",
             "fields": { "topics": "Укажите тему работы (минимум 3 символа)" } } }
```

- Codes: `validation_failed`, `not_found`, `maintenance`, `rate_limited`, `invalid_promo`,
  `unauthorized`, `conflict`, `internal`. Internal codes and stack traces are never exposed.
- Public endpoints hide operator-private fields (`contact`, `clientNote`, `operatorNotes`,
  `senderName`) unless the request carries the order token or an operator session.
- A wrong order/token pair always returns the same neutral `404` — no existence leaks.

---

## 9. HTTP API

### 9.1 Public

| Method & path | Purpose |
|---|---|
| `GET /api/health` | `{status, database, version, botActive}` |
| `GET /api/products` | Catalogue (names, unit words, ranges) |
| `POST /api/quote` | Price calculation (fast, p95 < 1 ms) |
| `POST /api/promos/check` | Promo validation with the exact messages (7.2) |
| `POST /api/auth/telegram/start` / `…/poll`, `GET /api/auth/me`, `POST /api/auth/logout` | Client sign-in through the bot |
| `POST /api/orders` | Create an order (**requires a client session**; `Idempotency-Key` supported; `503` in maintenance) |
| `GET /api/orders/{number}` | Public status page (`?token=` returns the full order) |
| `POST /api/orders/lookup` | `{number, token}` → `Order` (used by `/track`) |
| `POST /api/orders/batch` | `{items:[{number,token}]}` → the client's orders (profile page) |
| `POST /api/orders/{number}/claim` | «Я оплатил» → `payment_claimed` |
| `GET /api/orders/{number}/result` | Signed result download (`?sig=`, `?token=` or operator session) |
| `GET /api/reviews?status=published` | Public reviews |
| `POST /api/reviews` | Submit a review (`pending`) |
| `GET /api/settings`, `GET /api/settings/payment-qr` | Public settings incl. the Kaspi QR image |

### 9.2 Admin (`Authorization: Bearer <operator token>`)

| Method & path | Purpose |
|---|---|
| `POST /api/admin/auth/login` / `logout`, `GET /me`, `POST /auth/telegram/start` / `…/poll` | Session lifecycle (Telegram login is the UI path; password is break-glass) |
| `GET /api/admin/orders?status=&q=&sort=&page=&limit=&shape=` | Registry: `{orders, counts, total}`, **`payment_claimed` first** |
| `GET /api/admin/orders/{number}` | Full order (events, attachments, notes) |
| `POST /api/admin/orders/{number}/confirm` · `/reject` · `/cancel` · `/retry` | State transitions |
| `PATCH /api/admin/orders/{number}` | `{operatorNote}` (saved on blur) |
| `GET /api/admin/orders/{number}/console` (+ `/console/log`) | Live agent stream + raw log download |
| `GET /api/admin/reviews?status=` · `PATCH /api/admin/reviews/{id}` | Moderation |
| `GET/PUT /api/admin/settings` · `POST /api/admin/settings/payment-qr` | Settings + QR upload (multipart ≤ 1.5 MB, magic-byte checked) |
| `GET/PUT /api/admin/pricing` | Prices and volume tiers (validated) |
| `GET/POST /api/admin/promos` · `PATCH/DELETE /api/admin/promos/{id}` | Promo CRUD (duplicate codes rejected) |
| `GET /api/admin/stats` | Counts, revenue, queue depth, average generation time, failure rate, disk usage, oldest queued order (> 30 min → `queueWarning`) |

Middleware chain: CORS (`CORS_ORIGINS`), request logging, per-IP rate limiting (`429` +
`Retry-After`), session guard per route group, JSON envelope writer.

---

## 10. Telegram bot

### 10.1 Transport and delivery

- **Long polling** (`getUpdates`) — deliberately no webhook: the server needs no public URL or TLS.
- The bot runs inside the API process; `TELEGRAM_BOT_ENABLED=false` is fine — messages accumulate
  in `telegram_outbox` and deliver later.
- Operator chats are identified by `/bind @operator_almaty <password>` or the
  `TELEGRAM_ADMIN_CHAT_IDS` list. `/id` is a hidden helper that prints the chat id.

### 10.2 Client flows

| Flow | How it works |
|---|---|
| Sign-in (web) | The page shows a one-time code; the user sends `/login <code>` (or taps the deep link); the poll endpoint exchanges the confirmed challenge for a `{session, token}` pair. The chat becomes the client identity. |
| Order notifications | Orders created with the session are attached to the chat; **every** transition pushes a message (payment confirmed/rejected, queued, generating, completed with the file, cancelled, expired). |
| Manual linking | `DG-260391 CLS28G` (number + token) links an order created elsewhere to the chat. |
| `/status` | Lists the chat's orders with statuses. |
| `/start`, `/help` | Short instructions (no operator commands, no `/id`). |

### 10.3 Operator flows (bot)

| Command | Effect |
|---|---|
| `/orders` | Orders waiting for a payment check (`payment_claimed`) |
| `/order DG-260391` | Order card with the Kaspi sender name |
| `/confirm DG-260391` | `payment_claimed → queued` (goes straight to the agent) |
| `/reject DG-260391 <reason>` | `payment_claimed → payment_rejected` with a reason the client sees |
| `/cancel`, `/retry` | Cancel / requeue generation |
| `/pause`, `/resume` | Stop/start the generation queue (`settings.queuePaused`) |
| `/stats` | Orders per status, revenue, queue warning |
| `/login 123456` | Confirm a panel sign-in |

Inline buttons **✅ Подтвердить / ❌ Отклонить** appear on every payment-claim notification.
After a tap the message is rewritten without the buttons and shows who handled it — the chat
stays a clean audit trail. Buttons only work for operator chats; every action also writes an
`order_events` row.

### 10.4 Message conventions

- `parse_mode: HTML`; bold headline, then 2–4 short lines, then buttons (≤ 2 per row).
- One event = one message; no internal jargon; no access tokens in messages (the chat is the identity).
- The internal payment-claim message (operator):

```
💰 Оплата на проверку
DG-260391 · 800 ₸ · отправитель: Ануар А.
Реферат · 10 страниц · создан 17 окт, 14:05
[ ✅ Подтвердить ] [ ❌ Отклонить ]
```

---

## 11. Generation pipeline

### 11.1 Queue

- `queued` orders are claimed by the worker on a `WORKER_POLL_INTERVAL` (3 s) tick, up to
  `WORKER_MAX_AGENTS` (3) in parallel, and only when `settings.queuePaused = false`.
- Claiming flips the order to `generating` and opens an `order_jobs` row (attempt number, status, log).
- `WORKER_EXPIRE_AFTER=24h` — payment window for `awaiting_payment` orders.
- `WORKER_STALE_GENERATING_AFTER=30m` (never below the agent timeout) — orders stuck in
  `generating` are requeued; a live run is safe from being reaped.

### 11.2 Work directory anatomy

Each order gets a fresh directory `GENERATOR_DIR/work/<number>/`:

```
var/generator/work/DG-260396/
├── .clinerules/01-skill.md     # the skill instruction for the agent
├── skill/                      # skill references, templates, scripts
│   └── references/*.md         # e.g. document-structure, text-formatting, citations-references…
├── task.json                   # the order brief (product, topic, academic fields, requirements)
├── agent.log                   # raw agent stream, written live
└── result.docx / result.pptx   # the produced file (expected output)
```

### 11.3 Agent run

- Mode dispatch: `GENERATOR_MODE=agent` (always CLI), `auto` (CLI if the command resolves,
  otherwise built-in), `builtin` (bundled writer only — demo, no AI).
- The CLI runs with a pinned provider/model/thinking (`opencode-go` / `deepseek-v4.1-flash` / `high`
  by default), a per-order timeout (`GENERATOR_TIMEOUT=20m`; presentations may need more), and an
  optional BYOK key exported as the provider's env var.
- Skill selection follows the product: **satbayev-gost** for essay/lab/other, **pptx** for presentations.
- The prompt asks for the document only: no report, and the agent removes its own temporary files
  before finishing (only the result and the service files stay).

### 11.4 Output validation and storage

- The produced file is validated by its **ZIP/OOXML signature**, then moved into regular storage
  (`STORAGE_DIR`), an `order_files` row is written (`kind=result`, `expires_at = now + RESULT_TTL`),
  and the order flips to `completed`.
- The client downloads it via the signed URL and receives it as a Telegram document.
- The worker never trusts the extension alone — content is checked.

### 11.5 Failure, retries, and recovery

| Case | Behaviour |
|---|---|
| Agent failure | Retried up to `WORKER_AGENT_RETRIES` (default 1); attempts logged in `order_jobs` |
| Final failure | `failed_final`, agent log kept for the operator; `retry` from the panel/bot requeues |
| Agent CLI missing | `auto` mode falls back to the built-in `.docx` writer |
| Stuck in `generating` | Requeued by the housekeeping loop (threshold ≥ agent timeout) |
| Queue paused | Operators pause with `/pause`; the queue drains nothing new until `/resume` |

### 11.6 Operator console

`GET /api/admin/orders/{number}/console` renders the live agent stream into readable events
(iterations, tool commands, token usage, errors, final result); the admin order page polls it while
the status is `generating`. The raw stream is downloadable via `…/console/log`.

---

## 12. Frontend application

### 12.1 Routes

| Route | Screen |
|---|---|
| `/` | Landing: pricing, volume calculator, steps, perks, reviews |
| `/order` | Order form (sections А–I) + live price + promo + consent |
| `/order/:number` | Order status: Kaspi QR, «Я оплатил», progress, download |
| `/track` | Find an order by number + token |
| `/reviews` | Public reviews |
| `/profile` | The client's orders in this browser, review CTA, logout |
| `/legal` | Public offer, personal data, privacy policy |
| `/login` · `/admin/login` | Bot-mediated sign-in (client · operator) |
| `/admin/orders`, `/admin/orders/:number`, `/admin/reviews`, `/admin/promos`, `/admin/stats`, `/admin/pricing`, `/admin/settings` | Operator panel |

`src/app/router.tsx` uses `createBrowserRouter` with lazy-loaded pages, a `BootFallback`
hydration fallback, and `RouteError` boundaries. The data router is chosen specifically for
`useNavigation()` pending states, which drive the delayed route progress bar.

### 12.2 Module map

| Module | Responsibility |
|---|---|
| `lib/api.ts` | **Every** backend call; one function per endpoint; `ApiError` with the server envelope |
| `lib/session.ts` | Operator and client sessions in `localStorage`, `subscribeSession()`, the `srunal:unauthorized` event that a `401` dispatches |
| `lib/types.ts` | Domain types = the API contract |
| `lib/catalog.ts` | Products, institutes, languages, status metadata, file limits (static UI copy) |
| `lib/format.ts` | Money, dates, pluralisation |
| `lib/validate.ts` | Client-side form checks mirroring the server |
| `lib/pricing.ts` | Historical mirror of the formula (kept as a readable reference) |
| `lib/storage.ts`, `lib/seed.ts`, `lib/legal.ts`, `lib/env.ts`, `lib/theme.ts`, `lib/hooks.ts`, `lib/clipboard.ts`, `lib/assets.ts` | Persistence keys, demo data, legal texts, env, theming, hooks, clipboard, assets |

`localStorage` keys: `srunal.client.session` / `srunal.client.token`, `srunal.client.tokens`
(the `{number, token}` list this browser created), `srunal.admin.session` / `srunal.admin.token`,
plus UI-only keys (`srunal.theme`, form drafts, migration markers).

### 12.3 Theming and styling

- `styles/tokens.css` defines the full light and dark palettes as CSS custom properties
  (surfaces, text, brand blue `#1d4ed8`, statuses, radii, shadows, focus ring).
- `index.html` applies the stored/system theme **before first paint**; `lib/theme.ts` switches it
  with the View Transitions API cross-fade (`data-theme` attribute on `<html>`).
- `styles/components.css` and `pages.css` consume the tokens only; a `ThemeToggle` sits in the layout.

### 12.4 Sessions and 401 handling

- The admin stores `{session, token}` and sends `Authorization: Bearer <token>` on `/api/admin/*`;
  it re-checks via `GET /api/admin/me` on each panel load and drops the session on any `401`.
- Client sessions behave the same for order creation; a `401` clears the pair and the guard
  redirects to `/login` (or `/admin/login`).
- Result-download URLs are signed; the demo `/demo/resultat.docx` remains only as a fallback.

### 12.5 Live updates

- The order page polls every **8 s** while the status is `payment_claimed | queued | generating`
  and stops otherwise.
- The admin order page polls the console endpoint during generation.
- Toasts (`ToastProvider`) surface success/error feedback; `RouteProgress` shows navigation pending.

### 12.6 Admin panel screens

Orders registry (KPI cards, filters, pinned `payment_claimed`), order detail (receipt check,
confirm/reject, history, note, live Cline console), reviews moderation, promos CRUD, stats,
pricing editor, settings (Kaspi QR upload, recipient, support contact, maintenance, queue pause).

---

## 13. Security model

| Area | Implementation |
|---|---|
| Client auth | Bot-mediated one-time code → bearer session (30 days). Orders attach to the chat; no passwords. |
| Operator auth | Telegram login (primary) + Argon2id password with optional TOTP as a break-glass path; 5 wrong attempts → 15-minute lockout |
| Tokens | Order tokens and session tokens stored **hashed** with `TOKEN_PEPPER` (SHA-256 + pepper) |
| Sessions | Server-side records; only hashes stored; TTLs (`SESSION_TTL=12h`, `CLIENT_SESSION_TTL=720h`) |
| Rate limits | Orders 20/h/IP, reviews 10/h/IP, promo checks 60/min/IP → `429` + `Retry-After` |
| Uploads | Magic-byte content checks (not extensions), size caps, served with `Content-Disposition: attachment` |
| Privacy | Public order endpoint redacts operator notes, contacts, sender name; wrong pairs → identical neutral 404 |
| Payments | Manual by design; the client never proves payment; the operator confirms against the bank statement |
| CORS | Explicit allowlist (`CORS_ORIGINS`); `TRUST_PROXY` only behind a controlled reverse proxy |
| Error hygiene | No internal codes, worker details or stack traces in responses |
| Audit | Every operator action writes an `order_events` row; the bot chat keeps an edited-message trail |

---

## 14. Configuration reference

All settings live in `.env` (template: `.env.example`, every key commented).

| Group | Keys (defaults) |
|---|---|
| Core | `APP_ENV`, `HTTP_ADDR=:8080`, `DATABASE_URL`, `CORS_ORIGINS`, `PUBLIC_BASE_URL`, `FRONTEND_BASE_URL`, `TOKEN_PEPPER`, `TRUST_PROXY=false` |
| Sessions | `SESSION_TTL=12h`, `CLIENT_SESSION_TTL=720h`, `LOGIN_MAX_ATTEMPTS=5`, `LOGIN_LOCKOUT=15m` |
| Storage | `STORAGE_DIR=var/storage`, `RESULT_TTL=720h` (30 days) |
| Seed | `SEED_ON_START=true` (set `false` in production) |
| Rate limits | `RATE_LIMIT_ORDERS_PER_HOUR=20`, `RATE_LIMIT_REVIEWS_PER_HOUR=10`, `RATE_LIMIT_PROMO_PER_MIN=60` |
| Worker | `WORKER_ENABLED=true`, `WORKER_POLL_INTERVAL=3s`, `WORKER_EXPIRE_AFTER=24h`, `WORKER_STALE_GENERATING_AFTER=30m`, `WORKER_QUEUE_PAUSED_ON_START`, `WORKER_MAX_AGENTS=3`, `WORKER_AGENT_RETRIES=1` |
| Generation | `GENERATOR_MODE=agent\|auto\|builtin`, `GENERATOR_COMMAND=cline`, `GENERATOR_PROVIDER`, `GENERATOR_MODEL`, `GENERATOR_THINKING`, `GENERATOR_API_KEY(_ENV)`, `GENERATOR_CONFIG_DIR`, `GENERATOR_DATA_DIR`, `GENERATOR_TIMEOUT=20m`, `GENERATOR_DIR=var/generator` |
| Skills | `SKILL_SOURCE_DIR=~/.agents/skills`, `GENERATOR_SKILL_GOST=satbayev-gost`, `GENERATOR_SKILL_PPTX=pptx` |
| Telegram | `TELEGRAM_BOT_ENABLED`, `TELEGRAM_BOT_TOKEN`, `TELEGRAM_BOT_USERNAME=srunal_bot`, `TELEGRAM_ADMIN_CHAT_IDS`, `TELEGRAM_POLL_TIMEOUT=25s` |

`TOKEN_PEPPER` must be generated per deployment (`openssl rand -hex 32`) — changing it
invalidates existing tokens.

---

## 15. Running & testing

### 15.1 Local development (Windows/PowerShell, as documented in the READMEs)

```powershell
# 1. Database (once)
$env:PGPASSWORD="<postgres-password>"
& "C:\Program Files\PostgreSQL\18\bin\psql.exe" -h 127.0.0.1 -U postgres -f scripts/setup_db.sql
& "C:\Program Files\PostgreSQL\18\bin\psql.exe" -h 127.0.0.1 -U postgres -c "create database srunal owner srunal"

# 2. Backend
cd srunal_go
Copy-Item .env.example .env        # fill DATABASE_URL + TOKEN_PEPPER (+ bot token)
go run ./cmd/api                   # migrate + seed + serve on :8080

# 3. Frontend
cd ../srunal_vite
npm install
npm run dev                        # http://localhost:5173  (VITE_API_URL=http://localhost:8080)
```

Useful backend flags: `go run ./cmd/api -migrate` (migrations only), `-seed` (force demo import),
`-operator @operator_almaty -password 'NewSecret!2026'` (rotate a password).

### 15.2 Docker deployment (Ubuntu server)

```bash
cp .env.example .env     # fill the bot token; compose overrides DATABASE_URL
docker compose up -d     # postgres + api; migrations and seed run on start
docker compose logs -f api

# One-time agent authorization inside the container (persists in app-data):
docker compose exec -it api cline auth
# …or non-interactively:
docker compose exec api cline auth -p opencode-go -k <API_KEY> -m deepseek-v4.1-flash \
  --config /data/cline --data-dir /data/cline
```

### 15.3 Tests

```powershell
go test ./internal/...            # unit: quote, promo, validation, auth, docx, agent, limiter, bot commands
$env:TEST_DATABASE_URL="postgres://srunal:srunal@127.0.0.1:5432/srunal_test?sslmode=disable"
go test ./internal/store/...      # DB integration (skipped without TEST_DATABASE_URL)
powershell -File scripts/smoke.ps1   # 76 end-to-end HTTP checks (needs a running API)
npm run lint && npm run build     # frontend
```

The smoke test drives real flows (creating orders and reviews); point it at a throw-away database
or clean up with the SQL in the backend README.

### 15.4 Demo data (on a fresh database)

10 orders across the whole lifecycle, 11 reviews (6 published / 4 pending / 1 rejected),
6 promo codes, the pricing table, settings, and the demo operator
**`@operator_almaty` / `Eshkere9113!xxx!`**.

---

## 16. Deployment topology

```
Ubuntu server
├── docker compose
│   ├── db        postgres:17-alpine        volumes: db-data
│   └── api       srunal-api image          volumes: app-data (/data)
│       ├── :8080 HTTP API      (behind a reverse proxy for TLS)
│       ├── worker loop         (up to 3 agents in parallel)
│       ├── Telegram bot        (long polling — no inbound port needed)
│       └── /data
│           ├── storage/        uploaded + generated files (signed downloads)
│           ├── generator/      per-order work dirs + agent logs
│           └── cline/          agent CLI config + provider credentials
└── static SPA host or reverse-proxy route (the built srunal_vite bundle)
```

The container healthcheck calls `/api/health`;  the agent toolchain (Node 22, Cline, Python +
python-docx/pptx, pandoc) is baked into the image, so a payment confirmation can generate a
document end-to-end inside the container without host dependencies.

---

## 17. Known gaps & documented deviations

**Gaps (from the READMEs)**

- Payments stay manual — no Kaspi API integration.
- Cloudflare Turnstile is not wired in; rate limits are the defence.
- Client material is stored as **metadata only** (`attachments`); the real multipart upload
  endpoint is future work (the `order_files` schema already supports it).
- The generation worker needs the real skills tuned for full academic documents; the queue,
  jobs, retries and notifications are ready.
- i18n: UI is Russian; `kk`/`en` keys are planned.
- Observability: `GET /api/admin/stats` exists; OpenTelemetry/Prometheus are not configured.

**Documented deviations from the original frontend mock**

1. Seed access tokens are unique per order (PostgreSQL enforces it); API-created orders always use
   cryptographically random tokens.
2. `GET /api/orders/{number}` hides `contact`, `clientNote`, `operatorNotes`, `senderName`
   (the mock exposed everything).
3. Admin lists return `{orders, counts, total}` / `{reviews, counts}` envelopes; `?shape=array`
   gives plain arrays.
4. Extra fields the frontend can ignore: `resultFile`, `promoDropReason`.

---

## 18. The Telegram-first target

The next iteration moves the client experience fully into Telegram: **one bot** that sends all
notifications, plus **Mini Apps** (WebApps) opened from the bot for the actual flows. The website
is retired for clients; the operator web panel survives (and may become an operator Mini App later).

What the rebuild changes:

| Area | Today (website) | Target (Telegram-first) |
|---|---|---|
| Client entry | Landing page in a browser | Bot menu button → Mini App (short name `srunal`, deep links `?startapp=…`) |
| Client auth | One-time code via the bot | `initData` HMAC validation (`X-Telegram-Init-Data`) — no login screen at all |
| Order form | One long page (sections А–I) | 4-step Mini App wizard with `MainButton`/`BackButton`, haptics, draft persistence |
| Status tracking | `/order/:number` polling, `/track` with number+token | Order screen inside the Mini App (chat is the identity; `/track` disappears) |
| Look & feel | Custom brand styling (blue palette) | Telegram-native: `--tg-theme-*` variables, Day/Night themes, grouped lists, 14 px cards |
| Notifications | Same bot messages | Unchanged (already Telegram-first), extended with deep-link buttons into exact screens |

What is **reused as-is**: the Go backend (API, service core, state machine, worker, bot, outbox),
the PostgreSQL schema, the pricing/promo/validation rules, and most `lib/*` frontend modules.

The complete specification lives in:

- `../srunal_vite/design_md/design.md` — design tokens, components, 15 screens with wireframes,
  notification templates, states, accessibility, technical handoff, QA checklist.
- `../srunal_vite/design_md/prompt.md` — Google Stitch prompt kit that generates the Mini App
  screens (light + dark) from that spec.

---

## 19. Glossary

| Term | Meaning |
|---|---|
| **Order (заказ)** | One paid request; has a `DG-…` number, `works` (the number of separate works) and `units` (pages/slides per work) |
| **Work (работа)** | One document inside an order (e.g. 3 essays in one order) |
| **Units** | Pages for an essay, slides for a presentation; fixed products ignore it |
| **Claim (Я оплатил)** | The client's statement that the payment was sent; moves the order to `payment_claimed` |
| **Confirm / Reject** | The operator's verification of the payment against the bank statement |
| **Quote** | The server-computed price breakdown (`lines` array), frozen into the order at creation |
| **Queue** | The set of `queued` orders; picked up by the worker when `queuePaused = false` |
| **Agent** | The Cline CLI process that writes the document inside a per-order work dir |
| **Skill** | A folder with `SKILL.md` + references that instructs the agent (satbayev-gost, pptx) |
| **Outbox** | `telegram_outbox` table buffering notifications until the bot delivers them |
| **Kaspi QR** | The payment QR code (Kaspi.kz) uploaded by the operator in the admin settings |
| **СРС / СРСП** | Student independent work types (Kazakhstani academic terms) handled by the `other` product |

---

*End of document. Design of the Telegram Mini App: `../srunal_vite/design_md/design.md`.*
