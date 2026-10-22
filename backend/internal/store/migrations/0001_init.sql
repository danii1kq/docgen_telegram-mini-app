-- SruNal KZ - initial schema.
-- Mirrors README section 7.3 plus the tables the extra features need
-- (admin sessions, rate limiting, Telegram bot, idempotency keys).

create table if not exists schema_migrations (
  version    text primary key,
  applied_at timestamptz not null default now()
);

-- ---------------------------------------------------------------------------
-- catalogue & pricing
-- ---------------------------------------------------------------------------

create table if not exists product_pricing (
  product_code text primary key,          -- essay | lab | presentation | other
  min_units    int  not null check (min_units >= 1),
  max_units    int  not null check (max_units >= min_units),
  unit_price   int  not null check (unit_price > 0),
  active       boolean not null default true,
  updated_at   timestamptz not null default now()
);

create table if not exists volume_discounts (
  min_works int primary key check (min_works >= 1),
  pct       int not null check (pct between 1 and 90)
);

-- ---------------------------------------------------------------------------
-- orders
-- ---------------------------------------------------------------------------

create table if not exists orders (
  number           text primary key,               -- 'DG-260393'
  token_hash       text not null unique,           -- sha-256 of the 6-char access token
  status           text not null check (status in (
                     'awaiting_payment','payment_claimed','payment_rejected',
                     'queued','generating','completed',
                     'failed_final','expired','cancelled')),
  sender_name      text not null default '',
  product_code     text not null references product_pricing(product_code),
  works            int  not null check (works >= 1),
  units            int  not null check (units >= 1),
  topics           text not null,
  notes            text not null default '',
  -- Filled for "Другой док.": СРС, СӨЖ, практическая работа, РГР...
  work_kind        text not null default '',
  language         text not null check (language in ('ru','kk','en')),
  institute        text not null,
  department       text not null,
  student_name     text not null,
  discipline_code  text not null,
  teacher_name     text not null,
  contact_name     text not null,
  contact_type     text not null default 'telegram',
  contact          text not null default '',
  amount           int  not null check (amount >= 0),   -- frozen total, KZT
  quote_lines      jsonb not null default '[]'::jsonb,  -- frozen breakdown
  promo_code       text,
  promo_discount_pct int,
  client_note      text not null default '',
  operator_note    text not null default '',
  reject_reason    text,
  -- Telegram bot link: the chat that gets status push messages for this order.
  telegram_chat_id bigint,
  claimed_at       timestamptz,
  confirmed_at     timestamptz,
  completed_at     timestamptz,
  created_at       timestamptz not null default now(),
  updated_at       timestamptz not null default now()
);

create index if not exists orders_status_created_idx on orders (status, created_at desc);
create index if not exists orders_claimed_idx        on orders (created_at desc) where status = 'payment_claimed';
create index if not exists orders_chat_idx           on orders (telegram_chat_id) where telegram_chat_id is not null;
create index if not exists orders_created_idx        on orders (created_at desc);
create index if not exists orders_updated_idx        on orders (updated_at desc);

create table if not exists order_events (
  id           bigserial primary key,
  order_number text not null references orders(number) on delete cascade,
  actor        text not null check (actor in ('client','operator','system')),
  text         text not null,
  at           timestamptz not null default now()
);
create index if not exists order_events_order_idx on order_events (order_number, at desc);

create table if not exists order_files (
  id           bigserial primary key,
  order_number text not null references orders(number) on delete cascade,
  kind         text not null check (kind in ('client_material','result')),
  file_name    text not null,
  size_bytes   bigint not null,
  mime_type    text not null,
  storage_key  text not null default '',
  created_at   timestamptz not null default now(),
  expires_at   timestamptz                -- result files: 30 days
);
create index if not exists order_files_order_idx on order_files (order_number, kind, created_at desc);

create table if not exists order_jobs (               -- generation attempts / logs
  id           bigserial primary key,
  order_number text not null references orders(number) on delete cascade,
  attempt      int not null default 1,
  status       text not null check (status in ('queued','running','done','failed')),
  log          text,
  started_at   timestamptz,
  finished_at  timestamptz
);
create index if not exists order_jobs_order_idx on order_jobs (order_number, attempt desc);

-- ---------------------------------------------------------------------------
-- reviews
-- ---------------------------------------------------------------------------

create table if not exists reviews (
  id           text primary key,
  order_number text not null references orders(number),
  author       text not null,
  institute    text not null,
  department   text not null,
  discipline   text not null,
  product_code text not null,
  rating       numeric(2,1) not null check (rating between 1 and 5),
  text         text not null,
  status       text not null default 'pending' check (status in ('pending','published','rejected')),
  created_at   timestamptz not null default now(),
  moderated_at timestamptz,
  moderated_by text
);
create index if not exists reviews_status_created_idx on reviews (status, created_at desc);

-- ---------------------------------------------------------------------------
-- promo codes
-- ---------------------------------------------------------------------------

create table if not exists promo_codes (
  id           text primary key,
  code         text not null unique,
  discount_pct int not null check (discount_pct between 1 and 90),
  applies_to   text not null default 'any' check (applies_to in ('any','first')),
  min_amount   int,
  min_units    int,
  usage_limit  int,
  used_count   int not null default 0,
  expires_at   timestamptz,
  active       boolean not null default true,
  created_at   timestamptz not null default now()
);

create table if not exists promo_code_products (
  promo_id     text not null references promo_codes(id) on delete cascade,
  product_code text not null,
  primary key (promo_id, product_code)
);

create table if not exists promo_code_usages (
  id           bigserial primary key,
  promo_id     text not null references promo_codes(id),
  order_number text not null references orders(number),
  created_at   timestamptz not null default now(),
  unique (promo_id, order_number)
);

-- ---------------------------------------------------------------------------
-- settings (single row) & operators
-- ---------------------------------------------------------------------------

create table if not exists app_settings (
  id                int primary key default 1 check (id = 1),
  payment_qr_url    text,
  payment_recipient text not null default '',
  support_telegram  text not null default '',
  maintenance       boolean not null default false,
  queue_paused      boolean not null default false,
  requisites        jsonb not null default '{}'::jsonb
);

create table if not exists operators (
  id               text primary key,            -- '@operator_almaty'
  password_hash    text not null,               -- argon2id
  display_name     text not null default '',
  telegram_chat_id bigint,                      -- bound when the operator logs in via the bot
  active           boolean not null default true,
  created_at       timestamptz not null default now()
);

create table if not exists admin_sessions (
  token_hash   text primary key,                -- sha-256 of the bearer token
  operator_id  text not null references operators(id) on delete cascade,
  created_at   timestamptz not null default now(),
  expires_at   timestamptz not null,
  last_seen_at timestamptz not null default now(),
  ip           text not null default '',
  user_agent   text not null default ''
);
create index if not exists admin_sessions_operator_idx on admin_sessions (operator_id);

-- Rolling window of login attempts, used for the 15-minute lockout.
create table if not exists login_attempts (
  id         bigserial primary key,
  login_key  text not null,                     -- '<operator_id>|<ip>'
  successful boolean not null default false,
  at         timestamptz not null default now()
);
create index if not exists login_attempts_key_idx on login_attempts (login_key, at desc);

-- One-time code the operator sends to the bot to open a web session.
create table if not exists telegram_login_challenges (
  code         text primary key,               -- 6 digits
  kind         text not null default 'operator', -- operator | client
  operator_id  text not null default '',       -- prefilled handle, optional
  chat_id      bigint,
  chat_username text not null default '',
  created_at   timestamptz not null default now(),
  expires_at   timestamptz not null,
  confirmed_at timestamptz,
  used_at      timestamptz
);

-- ---------------------------------------------------------------------------
-- misc infrastructure
-- ---------------------------------------------------------------------------

create table if not exists counters (
  name  text primary key,
  value bigint not null
);

-- Double-submit protection: POST /api/orders with an Idempotency-Key header.
-- The original token is kept so a retry replays the same response; treat the
-- key as a client-side secret (a UUID), like payment providers do.
create table if not exists idempotency_keys (
  key          text primary key,
  order_number text not null,
  token        text not null default '',
  created_at   timestamptz not null default now()
);

-- Small key/value store (e.g. the Telegram getUpdates offset survives restarts).
create table if not exists app_kv (
  key        text primary key,
  value      text not null,
  updated_at timestamptz not null default now()
);

-- Client sessions opened through the Telegram bot (the chat id is the identity,
-- so orders created in the same browser attach to Telegram automatically).
create table if not exists client_sessions (
  token_hash   text primary key,
  chat_id      bigint not null,
  username     text not null default '',
  created_at   timestamptz not null default now(),
  expires_at   timestamptz not null,
  last_seen_at timestamptz not null default now(),
  ip           text not null default '',
  user_agent   text not null default ''
);
create index if not exists client_sessions_chat_idx on client_sessions (chat_id);

-- Outgoing Telegram messages, flushed by the bot loop with retries.
create table if not exists telegram_outbox (
  id           bigserial primary key,
  chat_id      bigint not null,
  text         text not null,
  order_number text,
  buttons      jsonb,                      -- inline keyboard (reply_markup)
  attempts     int not null default 0,
  last_error   text,
  created_at   timestamptz not null default now(),
  sent_at      timestamptz
);
create index if not exists telegram_outbox_pending_idx on telegram_outbox (sent_at, id);
