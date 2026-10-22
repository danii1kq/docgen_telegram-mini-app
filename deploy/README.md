# Deployment

Single container runs everything the product needs: the HTTP API, the Telegram
bot, the generation worker, the AI agent CLI and the built Mini App (served by
the API itself on the same origin).

> **Start here:** `VDS.md` — a step-by-step runbook for a fresh Ubuntu VDS with a
> free DuckDNS domain (Docker install → `.env` → `docker compose --profile tls up
> -d` → BotFather → opening the app inside Telegram). The sections below explain
> the pieces in more detail.

```
Ubuntu server
├── docker compose (this folder)
│   ├── db        postgres:17-alpine         volumes: db-data
│   ├── api       deploy/Dockerfile image    volumes: app-data (/data)
│   │   ├── :8080  API + Mini App + bot + worker + cline CLI   (loopback only)
│   │   └── /data  storage · generator work dirs · cline config
│   └── caddy     caddy:2-alpine (--profile tls)   :80 :443 → api:8080, auto-TLS
└── Internet/Telegram ── HTTPS ──► https://<DOMAIN>
```

## First run

```bash
cd backend && cp .env.example .env
#    fill TELEGRAM_BOT_TOKEN, TOKEN_PEPPER (openssl rand -hex 32),
#    TELEGRAM_BOT_USERNAME, PUBLIC_BASE_URL=https://<DOMAIN>
cd ../deploy && cp .env.example .env
#    fill DOMAIN (DuckDNS name), POSTGRES_PASSWORD, ACME_EMAIL (optional)
docker compose --profile tls up -d --build     # db + api + Caddy
docker compose logs -f api
```

Migrations and the demo seed (empty database only) run automatically on start;
set `SEED_ON_START=false` in production. Without the `tls` profile only `db` and
`api` start (the API is reachable on `127.0.0.1:8080`, useful for local
rehearsal).

`docker-compose.yml` overrides the host-oriented values of `backend/.env`
(`STORAGE_DIR`, `GENERATOR_DIR`, `GENERATOR_CONFIG_DIR`, `MINIAPP_DIR`,
`DATABASE_URL`, `APP_ENV`, `TRUST_PROXY`), so the same `.env` works for local
development and for the container. The build context is trimmed by
`../.dockerignore` (no `node_modules`, `dist`, `var`, `.env`).

## Agent authorization (once)

The generation worker runs the pinned Cline CLI on every confirmed order
(`opencode-go / deepseek-v4.1-flash`, thinking high). It needs credentials, kept
in the `app-data` volume:

```bash
# Option A — interactive login inside the container:
docker compose exec -it api cline auth

# Option B — BYOK: put the key in backend/.env, the API exports it to the agent
# under the provider's own variable (opencode-go -> OPENCODE_API_KEY):
#   GENERATOR_API_KEY=...
#   GENERATOR_PROVIDER=opencode-go
#   GENERATOR_MODEL=deepseek-v4.1-flash

# Option C — copy an existing authorization from a workstation:
docker compose cp ~/.cline/data/settings/providers.json api:/data/cline/settings/providers.json
```

Check the whole path without creating an order (development machine, needs the
same flags as the worker):

```bash
cd backend
go run ./cmd/agentcheck -cwd var/generator/work/DG-260391   # prints the CLI output
#   -command  agent CLI (default $GENERATOR_COMMAND or cline)
#   -provider/-model/-thinking  override the pinned model
```

The Dockerfile pins the CLI version and sets `CLINE_NO_AUTO_UPDATE=1`: a
self-update inside a running container replaces the binary under live workers
(observed in testing — the symlink disappeared mid-generation), so updates are
deployed by rebuilding the image instead.

`GENERATOR_MODE=agent` (the compose default) fails loudly when the model is
unavailable; set `GENERATOR_MODE=auto` in `backend/.env` to fall back to the
built-in `.docx` writer instead (demo mode, no AI).

## HTTPS and the Mini App URL

Telegram serves Mini Apps only over HTTPS. The bundled `caddy` service
(`--profile tls`) terminates TLS on ports 80/443 with Let's Encrypt — put your
domain into `deploy/.env` (`DOMAIN=srunal-kz.duckdns.org`) and Caddy issues the
certificate on the first start. Then in @BotFather:

1. `/newapp` (or Bot Settings → Configure Mini App) with the URL
   `https://your-domain.example`. This creates the **short name** — put the
   same value into `TELEGRAM_MINIAPP_SHORT_NAME` so notification buttons can
   deep-link (`https://t.me/<bot>/<short_name>?startapp=order_DG-…`).
2. Menu button → the same URL, so the app opens with one tap from the chat.
3. `/setcommands`: `start`, `help`, `status` for clients (+ `orders`, `stats`
   for operators).

Minimal Caddy (auto-TLS) in front of the compose stack — this is what the
`caddy` service runs, no host installation needed:

```
your-domain.example {
  encode gzip
  reverse_proxy api:8080
}
```

The service already does more: it reloads nothing (Caddy picks the config from
`deploy/Caddyfile`), replaces `X-Forwarded-For` with the real peer address (so
per-IP rate limits cannot be spoofed) and allows uploads up to 20 MB.

`PUBLIC_BASE_URL` in `backend/.env` must be that https origin: signed result
links and the Mini App deep links are built from it.

## Operators

Operator chats receive the payment-check messages and can work in the operator
Mini App (`Панель оператора` inside the app — the backend authorizes the panel
by the bound chat, no separate login):

```bash
# bind a chat to an operator from the bot itself (fastest):
/bind @operator_almaty <password>          # sent to the bot by that chat

# or list trusted chats in backend/.env and restart the api:
TELEGRAM_ADMIN_CHAT_IDS=123456789
```

## Updates

```bash
git -C ../.. pull
cd .. && docker compose -f deploy/docker-compose.yml up -d --build
```

From a Windows workstation without git, `deploy/push.ps1` does the same over SSH:
it packs the sources (never the `.env` files), uploads them, rebuilds the image
and restarts the stack:

```powershell
cd deploy
.\push.ps1 -Server root@<IP> -Domain srunal-kz.duckdns.org
```

Full options (manual upload, image transfer for weak servers, rollback) are in
`VDS.md §10`.

Back up `db-data` (orders) and `app-data` (storage, agent config/credentials).
