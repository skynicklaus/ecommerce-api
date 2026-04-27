-- +goose Up
-- +goose StatementBegin
SET lock_timeout = '2s';
ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE members ENABLE ROW LEVEL SECURITY;
ALTER TABLE member_roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE addresses ENABLE ROW LEVEL SECURITY;
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE customers ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;

ALTER TABLE organizations FORCE ROW LEVEL SECURITY;
ALTER TABLE members FORCE ROW LEVEL SECURITY;
ALTER TABLE member_roles FORCE ROW LEVEL SECURITY;
ALTER TABLE addresses FORCE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
ALTER TABLE customers FORCE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE ROW LEVEL SECURITY;

CREATE OR REPLACE FUNCTION is_platform_user() RETURNS bool AS $$
    SELECT current_setting('app.is_platform_user', true)::bool
  $$ LANGUAGE sql STABLE;

CREATE OR REPLACE FUNCTION is_platform_admin() RETURNS bool AS $$
    SELECT current_setting('app.is_platform_admin', true)::bool
  $$ LANGUAGE sql STABLE;

CREATE OR REPLACE FUNCTION current_org_id() RETURNS uuid AS $$
    SELECT current_setting('app.current_org_id', true)::uuid
  $$ LANGUAGE sql STABLE;

CREATE OR REPLACE FUNCTION current_identity_id() RETURNS uuid AS $$
    SELECT current_setting('app.current_identity_id', true)::uuid
  $$ LANGUAGE sql STABLE;

CREATE OR REPLACE FUNCTION is_member_of_current_org(p_member_id uuid) RETURNS bool AS $$
    SELECT EXISTS (
        SELECT 1 FROM members
        WHERE id = p_member_id
        AND organization_id = current_org_id()
    )
  $$ LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = pg_catalog, public;

CREATE POLICY org_isolation_read ON organizations
FOR SELECT
USING (
    type = 'merchant'
    OR id = current_org_id()
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON organizations
FOR INSERT
WITH CHECK (
    parent_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON organizations
FOR UPDATE
USING (
    id = current_org_id()
    OR is_platform_admin()
)
WITH CHECK (
    id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON organizations
FOR DELETE
USING (
    id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON members
FOR SELECT
USING (organization_id = current_org_id() OR is_platform_user());

CREATE POLICY org_isolation_write ON members
FOR INSERT
WITH CHECK (organization_id = current_org_id() OR is_platform_admin());

CREATE POLICY org_isolation_update ON members
FOR UPDATE
USING (organization_id = current_org_id() OR is_platform_admin())
WITH CHECK (organization_id = current_org_id() OR is_platform_admin());

CREATE POLICY org_isolation_delete ON members
FOR DELETE
USING (organization_id = current_org_id() OR is_platform_admin());

CREATE POLICY org_isolation_read ON member_roles
FOR SELECT
USING (
    is_member_of_current_org(member_id) OR is_platform_user()
);

CREATE POLICY org_isolation_write ON member_roles
FOR INSERT
WITH CHECK (
    is_member_of_current_org(member_id) OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON member_roles
FOR UPDATE
USING (
    is_member_of_current_org(member_id) OR is_platform_admin()
)
WITH CHECK (
    is_member_of_current_org(member_id) OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON member_roles
FOR DELETE
USING (
    is_member_of_current_org(member_id) OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON addresses
FOR SELECT
USING (
    organization_id = current_org_id() OR is_platform_user()
);

CREATE POLICY org_isolation_write ON addresses
FOR INSERT
WITH CHECK (
    organization_id = current_org_id() OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON addresses
FOR UPDATE
USING (
    organization_id = current_org_id() OR is_platform_admin()
)
WITH CHECK (
    organization_id = current_org_id() OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON addresses
FOR DELETE
USING (
    organization_id = current_org_id() OR is_platform_admin()
);

CREATE POLICY identity_isolation_read ON users
FOR SELECT
USING (
    identity_id = current_identity_id() OR is_platform_user()
);

CREATE POLICY identity_isolation_write ON users
FOR INSERT
WITH CHECK (
    identity_id = current_identity_id() OR is_platform_admin()
);

CREATE POLICY identity_isolation_update ON users
FOR UPDATE
USING (
    identity_id = current_identity_id() OR is_platform_admin()
)
WITH CHECK (
    identity_id = current_identity_id() OR is_platform_admin()
);

CREATE POLICY identity_isolation_delete ON users
FOR DELETE
USING (
    identity_id = current_identity_id() OR is_platform_admin()
);

CREATE POLICY identity_isolation_read ON customers
FOR SELECT
USING (
    identity_id = current_identity_id() OR is_platform_user()
);

CREATE POLICY identity_isolation_write ON customers
FOR INSERT
WITH CHECK (
    identity_id = current_identity_id() OR is_platform_admin()
);

CREATE POLICY identity_isolation_update ON customers
FOR UPDATE
USING (
    identity_id = current_identity_id() OR is_platform_admin()
)
WITH CHECK (
    identity_id = current_identity_id() OR is_platform_admin()
);

CREATE POLICY identity_isolation_delete ON customers
FOR DELETE
USING (
    identity_id = current_identity_id() OR is_platform_admin()
);

CREATE POLICY identity_isolation ON sessions
USING (identity_id = current_identity_id());

CREATE OR REPLACE FUNCTION admin_revoke_session(session_id uuid) RETURNS void
LANGUAGE sql SECURITY DEFINER
SET search_path = pg_catalog, public AS $$
    DELETE FROM sessions WHERE id = session_id
$$;

CREATE OR REPLACE FUNCTION admin_revoke_identity_sessions(p_identity_id uuid) RETURNS void
LANGUAGE sql SECURITY DEFINER
SET search_path = pg_catalog, public AS $$
    DELETE FROM sessions WHERE identity_id = p_identity_id
$$;

REVOKE ALL ON FUNCTION admin_revoke_session(uuid) FROM public;
REVOKE ALL ON FUNCTION admin_revoke_identity_sessions(uuid) FROM public;
GRANT EXECUTE ON FUNCTION admin_revoke_session(uuid) TO app_system;
GRANT EXECUTE ON FUNCTION admin_revoke_identity_sessions(uuid) TO app_system;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SET lock_timeout = '2s';
REVOKE EXECUTE ON FUNCTION admin_revoke_identity_sessions(uuid) FROM app_system;
REVOKE EXECUTE ON FUNCTION admin_revoke_session(uuid) FROM app_system;
DROP FUNCTION IF EXISTS admin_revoke_identity_sessions(uuid);
DROP FUNCTION IF EXISTS admin_revoke_session(uuid);
DROP POLICY IF EXISTS identity_isolation ON sessions;
DROP POLICY IF EXISTS identity_isolation_delete ON customers;
DROP POLICY IF EXISTS identity_isolation_update ON customers;
DROP POLICY IF EXISTS identity_isolation_write ON customers;
DROP POLICY IF EXISTS identity_isolation_read ON customers;
DROP POLICY IF EXISTS identity_isolation_delete ON users;
DROP POLICY IF EXISTS identity_isolation_update ON users;
DROP POLICY IF EXISTS identity_isolation_write ON users;
DROP POLICY IF EXISTS identity_isolation_read ON users;
DROP POLICY IF EXISTS org_isolation_delete ON addresses;
DROP POLICY IF EXISTS org_isolation_update ON addresses;
DROP POLICY IF EXISTS org_isolation_write ON addresses;
DROP POLICY IF EXISTS org_isolation_read ON addresses;
DROP POLICY IF EXISTS org_isolation_delete ON member_roles;
DROP POLICY IF EXISTS org_isolation_update ON member_roles;
DROP POLICY IF EXISTS org_isolation_write ON member_roles;
DROP POLICY IF EXISTS org_isolation_read ON member_roles;
DROP POLICY IF EXISTS org_isolation_delete ON members;
DROP POLICY IF EXISTS org_isolation_update ON members;
DROP POLICY IF EXISTS org_isolation_write ON members;
DROP POLICY IF EXISTS org_isolation_read ON members;
DROP POLICY IF EXISTS org_isolation_delete ON organizations;
DROP POLICY IF EXISTS org_isolation_update ON organizations;
DROP POLICY IF EXISTS org_isolation_write ON organizations;
DROP POLICY IF EXISTS org_isolation_read ON organizations;
DROP FUNCTION IF EXISTS is_member_of_current_org(uuid);
DROP FUNCTION IF EXISTS current_identity_id();
DROP FUNCTION IF EXISTS current_org_id();
DROP FUNCTION IF EXISTS is_platform_admin();
DROP FUNCTION IF EXISTS is_platform_user();
ALTER TABLE sessions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE customers NO FORCE ROW LEVEL SECURITY;
ALTER TABLE users NO FORCE ROW LEVEL SECURITY;
ALTER TABLE addresses NO FORCE ROW LEVEL SECURITY;
ALTER TABLE member_roles NO FORCE ROW LEVEL SECURITY;
ALTER TABLE members NO FORCE ROW LEVEL SECURITY;
ALTER TABLE organizations NO FORCE ROW LEVEL SECURITY;
ALTER TABLE sessions DISABLE ROW LEVEL SECURITY;
ALTER TABLE customers DISABLE ROW LEVEL SECURITY;
ALTER TABLE users DISABLE ROW LEVEL SECURITY;
ALTER TABLE addresses DISABLE ROW LEVEL SECURITY;
ALTER TABLE member_roles DISABLE ROW LEVEL SECURITY;
ALTER TABLE members DISABLE ROW LEVEL SECURITY;
ALTER TABLE organizations DISABLE ROW LEVEL SECURITY;
-- +goose StatementEnd
