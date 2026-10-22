# SruNal KZ — Telegram Mini App

React + Vite + TypeScript + Tailwind client for `@srunal_bot`, served by the Go
API on the same origin (`MINIAPP_DIR`). Styling follows the Stitch
"Telegram Mini App Design System" (see `../design/stitch/`), auth is
`initData`-only: every request carries `X-Telegram-Init-Data` and the backend
validates the HMAC with the bot token — no login code inside the app.

## Commands

```bash
npm install
npm run dev       # http://127.0.0.1:5174, /api proxied to :8080
npm run build     # tsc --noEmit && vite build → dist/
npm run preview   # static preview of the build
node scripts/i18n-check.mjs   # keys used in the client but missing in dict.ts
```

## Structure

```
src/
├── telegram/     # SDK wrapper + typings: theme, safe areas, MainButton/BackButton, haptics,
│                 # storage, links. Outside Telegram (or in a dev browser) the app renders its
│                 # own controls and shows the gate screen.
├── api/          # the single API seam: types.ts mirrors the Go domain; client.ts adds the
│                 # initData header, Idempotency-Key support and the ApiError envelope.
├── i18n/         # ru / kk / en: core.ts (detect, plural rules), dict.ts (translations keyed by
│                 # the Russian source), catalog.ts (product names, units, statuses), index.tsx
│                 # (provider + `t()` / `n()` helpers).
├── app/          # theme (auto / light / dark → data-scheme + data-theme-mode), session.
├── lib/          # format (₸ / Asia-Almaty dates / plurals, language-aware), catalog (statuses,
│                 # icons), draft (DeviceStorage persistence), legal (offer / personal / privacy), hooks.
├── components/   # Telegram-style UI kit: cards, inset rows, chips, stepper, sheet, toasts.
└── screens/      # Home · Wizard (4 steps) · Order (payment → status → result) · My orders ·
                  # Reviews · Support · Legal · Profile · Gate.
```

## Screens ↔ design

| Screen | Design | Source |
|---|---|---|
| Home (calculator, active orders, "Ещё") | S02 | `screens/HomeScreen.tsx` |
| Order wizard, steps 1–4 | S03–S06 | `screens/Wizard/**` |
| Order | S07–S08 | `screens/OrderScreen.tsx`, `screens/order/**` |
| My orders (filters, month groups) | S09 | `screens/MyOrdersScreen.tsx` |
| Reviews feed + form | S10–S11 | `screens/ReviewsScreen.tsx` |
| Support / FAQ | S12 | `screens/SupportScreen.tsx` |
| Legal documents | S13 | `screens/LegalScreen.tsx` |
| Profile | S14 | `screens/ProfileScreen.tsx` |
| Gate (outside Telegram) | S01 | `screens/GateScreen.tsx` |
| Operator panel (orders, reviews, promos, pricing, stats, settings) | S15 | `screens/operator/**` |

## Operator panel

The operator web panel of `srunal_vite` is rebuilt inside the Mini App on the
same design system (`/operator`): the order registry with `payment_claimed`
pinned, the receipt check (confirm/reject with reasons), retry/cancel, the
operator note, the order timeline and the **live generation console** (the same
`/api/admin/orders/{number}/console` stream), plus reviews moderation, promo
CRUD, the pricing editor, the KPI dashboard and the service settings (Kaspi QR
upload, maintenance, queue pause).

Authorization is the Telegram chat itself: the backend accepts the Mini App
`initData` on `/api/admin/*` when the chat is bound to an operator
(`/bind @operator_almaty <password>` in the bot, or `TELEGRAM_ADMIN_CHAT_IDS`).
`/api/auth/me` reports the operator identity, and the Home/Profile screens show
the entry point only for those chats.

## Local preview without Telegram

Telegram signs `initData`; a plain browser cannot. `scripts/dev-initdata.mjs`
mints a payload signed with the token from `../backend/.env`:

```bash
node scripts/dev-initdata.mjs             # prints auth_date=…&user=…&hash=…
# in the browser console:
#   localStorage.setItem('srunal.dev.initData', '<printed string>')
#   location.reload()
```

The override lives in `src/telegram/sdk.ts` and is compiled in **dev builds
only** (`import.meta.env.DEV`), so production always reads the signed SDK data.

`scripts/smoke-api.mjs` is an end-to-end HTTP check of the same flow (session,
quote, order creation, «Я оплатил» by chat identity, own-order read, neutral 404
for a foreign order) against a running backend.

## Behaviour notes

- **Payment**: the order screen leads with the operator's Kaspi QR link behind a
  red «Оплатить через Kaspi» button (`tg.openLink` → the Kaspi app), with the
  amount and recipient as real text; when no link is configured the block falls
  back to the uploaded QR image (`/api/settings/payment-qr`) or the design's
  vector plate. Both are operator settings (Панель оператора → Настройки →
  Оплата).

- **Theme**: follows the Telegram theme by default; Profile → Тема lets the user
  force light or dark (stored in DeviceStorage, applied as `data-theme-mode` on
  `<html>`; the base tokens come from the Telegram variables in `auto` and from
  fixed palettes in the forced modes, while the extension palette follows
  `data-scheme`). Safe areas stay computed from `contentSafeAreaInset + safeAreaInset`
  with hard fallbacks (notch + Telegram header), applied to the app shell only.

- **Language**: Profile → Язык интерфейса switches ru / kk / en (default from
  `tg.language_code`, stored in DeviceStorage). UI strings live in
  `src/i18n/dict.ts` keyed by the Russian source: an untranslated key simply
  falls back to Russian, so the dictionary can grow screen by screen
  (`node scripts/i18n-check.mjs` lists the gaps). Dates, money separators and
  plural forms follow the language through `lib/format.ts`; product names, unit
  words and order statuses through `i18n/catalog.ts`. Operator panel, the legal
  document bodies and the bot's chat messages stay Russian.
- **MainButton/BackButton**: native inside Telegram; in a browser the app
  renders equivalent inline controls (the design mockups).
- **Polling**: the order screen refreshes every 8 s while the status is
  `payment_claimed | queued | generating`.
- **Rework**: on a finished order the client can tap «Нужны правки», describe the
  corrections and send them to the operators (the bot also pings them). The
  operator sees the text in the panel («Доработка», a «Правки» chip in the
  orders list), may edit it and confirms with «Отправить на доработку» - the
  order goes back to `queued`, the agent regenerates the file with the
  corrections applied and the client gets a fresh document.
- **Multiple works**: an order with `works > 1` produces **one document per
  work** (`resultat_DG-…_1.docx`, `_2.docx`, …; the agent is asked for
  `result_1..N.docx` and must not merge the topics). The attempt fails when
  fewer documents arrive than ordered, the bot sends every document to the chat,
  and the order screen lists them with individual download links (the main
  button opens them one by one). The panel shows the same set and keeps the
  generation log and the final cost for finished orders.
- **Generation cost**: every agent run reports its cumulative cost (USD); the
  worker adds it to `orders.agent_cost_usd`, the operator sees
  «Затраты генерации: $…» on the order and the live console shows the running
  total.
- **Log viewer**: «Лог» in the console opens a signed short-lived link
  (`?token=…`, 12 h, HMAC with the bot token) in a plain browser - no Telegram
  session needed. The page appends new events without reloading and collapses
  long `run_commands` blobs.
- **Scroll**: every route change and every wizard step jumps the window back to
  the top (`useScrollTop` in `lib/hooks.ts`), so «Заказать» and «Далее» open the
  new screen at its heading instead of the previous scroll position.
- **Draft**: the wizard draft is written on every change *together with the
  step*: reopening the wizard continues where the client stopped (with a
  «Черновик восстановлен» cue) and starting an order with a saved draft asks
  «Продолжить или начать заново». Writes go to both `localStorage` and
  `tg.DeviceStorage` (Bot API 9.0), reads prefer the synchronous one, so the
  draft, the templates, the theme and the language survive on every client.
- **Templates** («Мои шаблоны», `/templates`): title-page sets (institute,
  department, student, programme code, teacher) are saved on device from wizard
  step 3 and can be applied to the draft or deleted from that screen.
- **Institutes / departments**: the lists mirror the university's own pages
  (`constants.ts`); both pickers end with «Другое (вписать свой вариант)», which
  reveals a free-text field, so any institute or department can be entered.
- **Dynamic lists**: the draft is cleared only on a successful order.
