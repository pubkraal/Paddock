-- 0004_identity_lookup_fn — the one sanctioned RLS bypass (ADR-0012).
--
-- At login the user types only their email; we do not yet know their org, so we
-- cannot SET app.current_org and an RLS-protected SELECT on users returns zero
-- rows. This SECURITY DEFINER function, owned by the migration role (which is
-- BYPASSRLS), resolves email -> identity across tenants, returning ONLY the four
-- columns login needs. EXECUTE is granted solely to paddock_app; search_path is
-- pinned so the definer-rights function cannot be hijacked.
--
-- Anti-enumeration is the caller's responsibility: the login handler always
-- responds identically whether or not a row is returned.

CREATE FUNCTION identity_lookup(p_email citext)
RETURNS TABLE (user_id uuid, org_id uuid, role user_role, status user_status)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
    SELECT id, org_id, role, status
    FROM users
    WHERE email = p_email;
$$;

REVOKE ALL ON FUNCTION identity_lookup(citext) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION identity_lookup(citext) TO paddock_app;
