DROP POLICY IF EXISTS org_isolation_update ON users;
DROP POLICY IF EXISTS org_isolation_insert ON users;
DROP POLICY IF EXISTS org_isolation_select ON users;
DROP TABLE IF EXISTS users;
DROP TYPE IF EXISTS user_status;
DROP TYPE IF EXISTS user_role;
