-- Inline buttons for bot notifications.
--
-- The "payment claimed" message operators receive carries two buttons:
-- «Подтвердить» (queue the order for generation) and «Отклонить» (tell the
-- client the payment was not found). The keyboard is stored with the message so
-- a queued notification keeps its buttons after a restart.

alter table telegram_outbox
  add column if not exists buttons jsonb;
