-- Two-role convention (ADR-0008 / ADR-0009).
--   paddock_migrate : owns the schema, BYPASSRLS — used by golang-migrate only.
--   paddock_app     : the application role; RLS policies apply to it, never a superuser.
-- This script runs once, as the postgres superuser, on first container init.

CREATE ROLE paddock_migrate LOGIN PASSWORD 'paddock_migrate' BYPASSRLS;
CREATE ROLE paddock_app LOGIN PASSWORD 'paddock_app';

ALTER DATABASE paddock OWNER TO paddock_migrate;

GRANT ALL ON SCHEMA public TO paddock_migrate;
GRANT USAGE ON SCHEMA public TO paddock_app;

-- Objects created by the migration role are readable/writable by the app role.
ALTER DEFAULT PRIVILEGES FOR ROLE paddock_migrate IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO paddock_app;
ALTER DEFAULT PRIVILEGES FOR ROLE paddock_migrate IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO paddock_app;
