-- A promoted reader keeps their profile and loan history.
ALTER TABLE accounts DROP CONSTRAINT accounts_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_reader_required CHECK(role <> 'reader' OR reader_id IS NOT NULL);
