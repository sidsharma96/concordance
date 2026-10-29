-- One-shot psql bootstrap, run as an administrator connected to postgres.
-- Existing names fail explicitly; partial setup requires inspection, not rerunning.
\set ON_ERROR_STOP on
DO $$
BEGIN
    IF current_database() <> 'postgres' THEN
        RAISE EXCEPTION 'connect to postgres for bootstrap';
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname IN ('concordance_owner', 'concordance_app'))
       OR EXISTS (SELECT 1 FROM pg_database WHERE datname = 'concordance') THEN
        RAISE EXCEPTION 'Concordance names already exist; inspect before proceeding';
    END IF;
END $$;
CREATE ROLE concordance_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
CREATE ROLE concordance_app NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 4;
CREATE DATABASE concordance OWNER concordance_owner ALLOW_CONNECTIONS false;
REVOKE ALL ON DATABASE concordance FROM PUBLIC;
ALTER DATABASE concordance ALLOW_CONNECTIONS true;
\connect concordance
REVOKE ALL ON SCHEMA public FROM PUBLIC;
CREATE SCHEMA catalog AUTHORIZATION concordance_owner;
SET ROLE concordance_owner;
SET search_path = catalog;
\ir ../../internal/catalog/schema.sql
\ir ../../internal/catalog/receipts.sql
RESET ROLE;
\ir grants.sql
ALTER ROLE concordance_app IN DATABASE concordance SET search_path = catalog;
-- Install and verify the user-specific HBA include before setting a password
-- and enabling LOGIN. Neither action is part of this file.
