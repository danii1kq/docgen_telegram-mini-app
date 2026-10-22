-- Result files come in sets.
--
-- An order with several works produces one document per work, and a rework
-- replaces the previous set. `attempt` groups the files of one generation run,
-- so the client and the panel always see the newest set (the older sets stay on
-- disk for the operator history).

alter table order_files add column if not exists attempt int not null default 1;

create index if not exists order_files_result_idx
  on order_files (order_number, attempt desc, id);
