-- Add named targets to an existing Concordance catalog, using psql as its administrator.
-- This is one-shot setup, not bootstrap or an automatic runtime migration.
\set ON_ERROR_STOP on
BEGIN;
DO $$
BEGIN
    IF current_database() <> 'concordance' THEN
        RAISE EXCEPTION 'connect to concordance for target setup';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_namespace
                   WHERE nspname = 'catalog'
                     AND pg_get_userbyid(nspowner) = 'concordance_owner') THEN
        RAISE EXCEPTION 'expected Concordance catalog owner is absent';
    END IF;
END $$;
SET LOCAL ROLE concordance_owner;
SET LOCAL search_path = catalog;
\ir ../../internal/catalog/targets.sql
REVOKE ALL ON catalog.concordance_search_targets FROM PUBLIC, concordance_app;
\ir target-grants.sql
COMMIT;
