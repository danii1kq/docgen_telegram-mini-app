-- The review table no longer stores the "final grade" (grade) of the work:
-- clients submit only the star rating and the text. This migration removes the
-- column from databases created before the change (fresh installations never
-- create it - see 0001_init.sql).

alter table reviews drop column if exists grade;
