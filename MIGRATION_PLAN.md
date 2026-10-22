# Migration Plan — website + bot → Telegram-first

> **Status:** PR 1–4 implemented in this repository · PR 5 (ship) specified below
> **Date:** 17 October 2026
> **Inputs:** `../STACK.md` (how the current system works), `../srunal_vite/design_md/design.md`
> (Mini App design spec), `../srunal_vite/design_md/prompt.md` (Stitch prompt kit),
> `design/stitch/` (the generated Stitch screens)

## 1. Principles

1. **Evolve, do not rewrite.** The Go core (state machine, worker, agent pipeline, bot,
   outbox, DB) is reused as-is. The pivot touches the entry point (initData auth), the static
   hosting and the notification copy — not the engine.
2. **One origin.** The API serves the built Mini App itself, so `initData` requests are
   same-origin: no CORS, no second deployment unit.
3. **The website is retired for clients.** The operator web panel stays (its routes and the
   `FrontendBaseURL` links in operator messages remain until the operator Mini App ships).
4. **Every PR ships runnable.** No long-lived branch that breaks login or the bot.

## 2. What moves where

| Old location | New location | Change |
|---|---|---|
| `srunal_go/**` | `backend/**` | Moved; modified only where the Telegram-first flow needs it |
| `srunal_vite/src/{lib,styles,catalog,format,validate}` | reference for `miniapp/src` | Reused as *copy/pattern*, not imported (the Mini App is a separate Vite app) |
| `srunal_vite/src/pages/*` (client) | `miniapp` screens (PR 2+) | Rewritten to the Mini App spec; the website pages stay in git history only |
| `srunal_vite/src/pages/admin/*` | unchanged in `srunal_vite` | The panel keeps working against the same API |
| `srunal_vite/design_md/**` | stays | Design source of truth for the app |
| `srunal_go/docker-compose.yml` + `Dockerfile` | `deploy/**` | New image builds backend + Mini App together |

## 3. PR roadmap

### PR 1 — Foundation: initData auth + SPA hosting ✅ (this repository)

Delivered:

| Change | Where |
|---|---|
| `initData` validation (HMAC-SHA256, `auth_date` freshness, both `signature` variants) + tests | `backend/internal/auth/initdata.go`, `initdata_test.go` |
| Chat identity from `X-Telegram-Init-Data` without a session row | `backend/internal/service/auth_client.go` (`ClientSessionFromInitData`) |
| The client middleware accepts bearer **or** initData on every guarded route | `backend/internal/api/server.go` (`clientSession`, CORS header list) |
| Mini App SPA served on non-`/api` routes with immutable asset cache + SPA fallback | `backend/internal/api/miniapp.go`, router tail in `server.go` |
| Config: `MINIAPP_DIR`, `TELEGRAM_MINIAPP_SHORT_NAME`, `Config.MiniAppLink()` deep-link builder | `backend/internal/config/config.go`, `.env.example` |
| Bot notifications carry "open in the app" buttons when the deep link is configured | `backend/internal/service/notify.go` (completed, payment confirmed) |
| Mini App scaffold: Telegram bootstrap, theme tokens, API seam, Home screen with live quote, gate screen | `miniapp/**` |
| Single-image deploy (backend + built app) | `deploy/**` |

**Acceptance:** `go build ./... && go test ./internal/...` green; app opens from a tunnel
inside Telegram, shows the user's first name, resolves `/api/auth/me` through a signed header;
the Home quote matches the server; the gate screen appears in a plain browser.

### PR 2 — Order wizard (design S03–S06) ✅

- Screens: 4-step wizard with the progress header, option cards, steppers, sheet pickers,
  segmented language control, files (metadata only), promo field, price breakdown, consent.
- Data: `POST /api/orders` called with initData + `Idempotency-Key`; `error.fields` map onto
  inputs and the first invalid input is focused/scrolled into view.
- `MainButton` = "Далее → / Перейти к оплате · N ₸", `BackButton` = previous step, closing
  confirmation while the draft is dirty.
- Draft in `tg.DeviceStorage` (fallback `localStorage`), quote debounced 300 ms server-side.
- `/api/products` now carries the live unit price + short unit label so the calculator shows
  the server price; `POST /api/quote` accepts the applied promo and returns the discounted total.

**Delivered in `miniapp/src/screens/Wizard/**`.**

### PR 3 — Order screens, deep links, notifications ✅

- S07 payment (Kaspi QR card + sender name + «Я оплатил» → `…/claim`), S08 status
  (stepper, timeline, contextual MainButton, 8 s polling while active), completed result card
  (`tg.openLink` on the signed URL).
- `startapp` routing: `order_<number>`, `new`, `orders`, `support`, `review_<number>` —
  read `initDataUnsafe.start_param` once and `replace` it away.
- New endpoint `GET /api/me/orders`: the chat's orders from initData, newest first,
  `{orders, counts}` envelope. `GET /api/orders/{number}`, `POST …/claim` and
  `GET …/result` now also accept the chat identity (no number+token pairs in the app);
  a foreign order answers the neutral 404.
- Notification templates: the bot keeps its current copy; the `design.md §8` alignment
  (extra buttons) stays in PR 5.

### PR 4 — Reviews, support, legal, profile (S10–S14) ✅

- Review form (rating stars + text, moderated, de-duplicated per order server-side),
  reviews feed (published only), support/FAQ screen with `openTelegramLink`, legal documents
  rendered from the moved `legal.ts` copy, profile screen (identity card, draft reset, version).

**Delivered in `miniapp/src/screens/ReviewsScreen.tsx`, `SupportScreen.tsx`,
`LegalScreen.tsx`, `ProfileScreen.tsx`.**

### PR 5 — Ship

- BotFather: `/newapp` short name + menu button + `/setcommands`.
- TLS reverse proxy in front of the container (`deploy/README.md`).
- Bot notification templates ✅ aligned with `design.md §8.2` (templates 2–9, buttons
  «Перейти к оплате», «Я оплатил ещё раз», «Оставить отзыв», «Поддержка»; the finished
  document arrives with `sendDocument`).
- Retire client routes of the website (keep `/admin/*`); update `FRONTEND_BASE_URL` usage in
  operator messages to the panel origin only.
- Update `STACK.md` and the backend `README.md` sections that describe the old login flow.
- QA per `design.md §14` on iOS, Android, Desktop.

### PR 6 — Operator Mini App (design S15) ✅

The operator section of the Mini App replaces the web panel for everyday work:
registry with `payment_claimed` pinned and KPI counters, order card (Kaspi sender,
receipt check with confirm/reject reasons, retry/cancel, operator note, audit
timeline, **live generation console**, result download), reviews moderation,
promo CRUD, pricing editor, KPI dashboard and service settings (Kaspi QR upload,
maintenance, queue pause).

Authorization reuses the chat identity: `requireAdmin` accepts the validated
initData of a chat bound to an operator (`AdminSessionFromInitData`), and
`/api/auth/me` returns the operator so the app shows the entry. The web panel
keeps working against the same API for as long as it is deployed.

**Acceptance:** production QA checklist (`design.md §14`) passes on iOS, Android, Desktop.

### PR 6 (optional) — Operator Mini App (design S15)

Queue list with pinned `payment_claimed`, order card, confirm/reject sheets, stats, queue
pause — reusing the same design system. Until then operators use the bot buttons and the web
panel.

## 4. API surface changes

| Endpoint | Change |
|---|---|
| *(all client routes)* | Accept `X-Telegram-Init-Data` in addition to `Authorization: Bearer` (done in PR 1) |
| `GET /api/auth/me` | Returns the chat identity for initData requests (done in PR 1) |
| `POST /api/orders` | Unchanged; called with the header + `Idempotency-Key` from the app (PR 2) |
| `GET /api/products` | Carries the live unit price + `unitLabel` (PR 2) |
| `POST /api/quote` | Accepts `promo: {code}`; returns the discounted total (PR 2) |
| `GET /api/me/orders` | **New** in PR 3: the chat's orders (no number+token pairs) |
| `GET /api/orders/{number}` | With a client session: the full own order, neutral 404 for a foreign one (PR 3) |
| `POST /api/orders/{number}/claim` | Without a token: the chat identity authorizes the claim (PR 3) |
| `GET /api/orders/{number}/result` | Also authorized for the owning chat (PR 3) |
| `POST /api/reviews` | Unchanged; called with the header (PR 4) |

## 5. What gets deleted / demoted

| Item | Action |
|---|---|
| Website client routes (`/`, `/order`, `/track`, `/profile`, `/reviews`, `/legal`, `/login`) | Retired in PR 5 (admin routes stay) |
| Client one-time-code login (`/api/auth/telegram/start`/`poll`) | Stays for the operator panel login; the client path becomes unused by the app |
| `FRONTEND_BASE_URL` links inside bot messages | Replaced by Mini App deep links where a screen exists |
| `srunal_go` root folder | Frozen; development continues in `srunal_tgbot_miniapp/backend` |

## 6. Test strategy

| Layer | Addition |
|---|---|
| Unit | `initdata_test.go` (done): signature, both `signature` variants, tampering, expiry, wrong token |
| Service | PR 2: order creation with an initData identity (fake store pattern already used in `service/*_test.go`) |
| Integration | PR 3: `/api/me/orders` against a throw-away DB (`TEST_DATABASE_URL`) |
| Smoke | PR 5: locally-signed initData block in `scripts/smoke.ps1` |
| Manual | Per-PR slice of `design.md §14` (theming on iOS/Android, MainButton states, keyboard, safe areas) |

## 7. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Keyboard-button launches have **no** `initData` | The gate screen explains how to open the app properly (already handled) |
| Signature-variant differences across clients | Validator accepts both (with and without `signature` in the check string) |
| `auth_date` freshness vs. long sessions | 24 h TTL + re-check per request; the app refreshes initData on every open |
| Deep link charset limits (`A-Z a-z 0-9 _ -`) | Order numbers (`DG-260391`) are already compatible |
| iOS file picker quirks (PR 2 attachments) | Metadata-only for now; real uploads stay behind the multipart endpoint |
| Result download on Telegram Web | `Content-Disposition: attachment` + `https://web.telegram.org` in CORS (already documented in `STACK.md §17`) |
| Lost orders during cut-over | Both auth paths run in parallel until PR 5; the bot keeps notifying regardless of the surface |

## 8. Rollback

PR 1–4 are additive: the website keeps working, the API keeps accepting bearer tokens, the
gate screen only appears outside Telegram. Reverting a PR is a `git revert`; no schema
migrations are involved. PR 5 (retiring client routes) is the only step that needs the
shorter rollback window — keep the previous image tag for one deploy cycle.
