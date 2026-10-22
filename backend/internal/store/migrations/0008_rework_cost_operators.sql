-- Rework, per-order agent cost and multiple operator chats.
--
--   * operator_chats: an operator account may be bound to several Telegram
--     chats and several operators coexist - a new /bind no longer unlinks the
--     chat of another operator (before this table one chat per operator column
--     was the only link).
--   * revision_note / revision_requested_at: the client asks for corrections on
--     a finished order, the operator edits them and sends the work back to the
--     agent with the final wording.
--   * agent_cost_usd: what the generation runs of this order cost in USD
--     (summed over attempts), shown in the operator panel.

create table if not exists operator_chats (
  chat_id     bigint primary key,
  operator_id text not null references operators(id) on delete cascade,
  created_at  timestamptz not null default now()
);
create index if not exists operator_chats_operator_idx on operator_chats (operator_id);

insert into operator_chats (chat_id, operator_id)
select o.telegram_chat_id, o.id
  from operators o
 where o.telegram_chat_id is not null
on conflict (chat_id) do nothing;

alter table orders add column if not exists revision_note text not null default '';
alter table orders add column if not exists revision_requested_at timestamptz;
alter table orders add column if not exists agent_cost_usd numeric(12, 4) not null default 0;

create index if not exists orders_revision_idx on orders (revision_requested_at)
  where revision_requested_at is not null;
