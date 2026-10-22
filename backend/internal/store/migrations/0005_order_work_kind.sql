-- The order form now asks what exactly the work is when "Другой док." is chosen
-- (СРС, СӨЖ, практическая работа, РГР, эссе...). The value goes to the title page
-- and to the generation task, so the agent knows the required structure.

alter table orders add column if not exists work_kind text not null default '';
