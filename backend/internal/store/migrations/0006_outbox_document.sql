-- Result documents delivered into the chat.
--
-- The finished document is the primary delivery path of an order (design.md
-- §8.2 template 6): the notification queue therefore carries the stored file
-- path and its display name next to the message text, and the bot loop sends
-- it with sendDocument. The signed URL stays in the caption as the fallback.

alter table telegram_outbox
  add column if not exists document_path text not null default '',
  add column if not exists document_name text not null default '';
