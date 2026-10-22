-- Client authorization through the Telegram bot, plus the removal of the 2FA
-- (TOTP) feature that belonged to the password login.
--
-- Flow: the site asks for a one-time code (POST /api/auth/telegram/start), the
-- client sends /login <code> to the bot, the chat is recorded on the challenge
-- and the site exchanges the code for a client session. The chat id is the
-- client identity, so orders created in that browser are attached to Telegram
-- automatically. The same mechanism opens an operator panel session when the
-- chat is bound to an operator.

create table if not exists client_sessions (
  token_hash   text primary key,          -- sha-256 of the bearer token
  chat_id      bigint not null,           -- Telegram identity of the client
  username     text not null default '',  -- @handle, when the client has one
  created_at   timestamptz not null default now(),
  expires_at   timestamptz not null,
  last_seen_at timestamptz not null default now(),
  ip           text not null default '',
  user_agent   text not null default ''
);
create index if not exists client_sessions_chat_idx on client_sessions (chat_id);

-- The challenge knows which session it opens: an operator panel session or a
-- client session (see the note above).
alter table telegram_login_challenges
  add column if not exists kind text not null default 'operator';
alter table telegram_login_challenges
  add column if not exists chat_username text not null default '';

-- 2FA is gone: the panel is entered through the bot only.
alter table operators drop column if exists totp_secret;
