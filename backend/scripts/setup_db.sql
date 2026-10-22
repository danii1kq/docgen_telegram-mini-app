-- SruNal KZ - database bootstrap.
-- Run as a PostgreSQL superuser:
--   psql -h 127.0.0.1 -U postgres -f scripts/setup_db.sql
-- The API applies its own migrations on startup (internal/store/migrations).

do $$
begin
  if not exists (select 1 from pg_roles where rolname = 'srunal') then
    create role srunal with login password 'srunal';
  else
    alter role srunal with login password 'srunal';
  end if;
end
$$;

-- CREATE DATABASE cannot run inside a transaction/DO block; run it separately
-- if the database does not exist yet:
--   psql -h 127.0.0.1 -U postgres -c "create database srunal owner srunal"
select 'role srunal is ready; create the database with: create database srunal owner srunal'
  where not exists (select 1 from pg_database where datname = 'srunal');
