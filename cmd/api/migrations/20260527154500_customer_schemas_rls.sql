-- +goose Up
-- +goose StatementBegin
SET
    lock_timeout = '2s';

ALTER TABLE
    carts ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    cart_shop_groups ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    cart_items ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    carts FORCE ROW LEVEL SECURITY;

ALTER TABLE
    cart_shop_groups FORCE ROW LEVEL SECURITY;

ALTER TABLE
    cart_items FORCE ROW LEVEL SECURITY;

CREATE
OR REPLACE FUNCTION is_current_org_cart(p_cart_id uuid) RETURNS bool AS
$$
SELECT
    EXISTS (
        SELECT
            1
        FROM
            carts
        WHERE
            id = p_cart_id
            AND customer_org_id = current_org_id()
    )
$$
LANGUAGE SQL STABLE SECURITY DEFINER
SET
    search_path = pg_catalog,
    public;

CREATE
OR REPLACE FUNCTION is_current_org_cart_shop_group(p_cart_shop_group_id uuid) RETURNS bool AS
$$
SELECT
    EXISTS (
        SELECT
            1
        FROM
            cart_shop_groups g
            JOIN carts c ON c.id = g.cart_id
        WHERE
            g.id = p_cart_shop_group_id
            AND c.customer_org_id = current_org_id()
    )
$$
LANGUAGE SQL STABLE SECURITY DEFINER
SET
    search_path = pg_catalog,
    public;

CREATE POLICY org_isolation_read ON carts FOR
SELECT
    USING (
        customer_org_id = current_org_id()
        OR is_platform_user()
    );

CREATE POLICY org_isolation_write ON carts FOR
INSERT
    WITH CHECK (
        customer_org_id = current_org_id()
        OR is_platform_admin()
    );

CREATE POLICY org_isolation_update ON carts FOR
UPDATE
    USING (
        customer_org_id = current_org_id()
        OR is_platform_admin()
    ) WITH CHECK (
        customer_org_id = current_org_id()
        OR is_platform_admin()
    );

CREATE POLICY org_isolation_delete ON carts FOR DELETE USING (
    customer_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON cart_shop_groups FOR
SELECT
    USING (
        is_current_org_cart(cart_id)
        OR is_platform_user()
    );

CREATE POLICY org_isolation_write ON cart_shop_groups FOR
INSERT
    WITH CHECK (
        is_current_org_cart(cart_id)
        OR is_platform_admin()
    );

CREATE POLICY org_isolation_update ON cart_shop_groups FOR
UPDATE
    USING (
        is_current_org_cart(cart_id)
        OR is_platform_admin()
    ) WITH CHECK (
        is_current_org_cart(cart_id)
        OR is_platform_admin()
    );

CREATE POLICY org_isolation_delete ON cart_shop_groups FOR DELETE USING (
    is_current_org_cart(cart_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON cart_items FOR
SELECT
    USING (
        is_current_org_cart_shop_group(cart_shop_group_id)
        OR is_platform_user()
    );

CREATE POLICY org_isolation_write ON cart_items FOR
INSERT
    WITH CHECK (
        is_current_org_cart_shop_group(cart_shop_group_id)
        OR is_platform_admin()
    );

CREATE POLICY org_isolation_update ON cart_items FOR
UPDATE
    USING (
        is_current_org_cart_shop_group(cart_shop_group_id)
        OR is_platform_admin()
    ) WITH CHECK (
        is_current_org_cart_shop_group(cart_shop_group_id)
        OR is_platform_admin()
    );

CREATE POLICY org_isolation_delete ON cart_items FOR DELETE USING (
    is_current_org_cart_shop_group(cart_shop_group_id)
    OR is_platform_admin()
);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP POLICY IF EXISTS org_isolation_delete ON cart_items;

DROP POLICY IF EXISTS org_isolation_update ON cart_items;

DROP POLICY IF EXISTS org_isolation_write ON cart_items;

DROP POLICY IF EXISTS org_isolation_read ON cart_items;

DROP POLICY IF EXISTS org_isolation_delete ON cart_shop_groups;

DROP POLICY IF EXISTS org_isolation_update ON cart_shop_groups;

DROP POLICY IF EXISTS org_isolation_write ON cart_shop_groups;

DROP POLICY IF EXISTS org_isolation_read ON cart_shop_groups;

DROP POLICY IF EXISTS org_isolation_delete ON carts;

DROP POLICY IF EXISTS org_isolation_update ON carts;

DROP POLICY IF EXISTS org_isolation_write ON carts;

DROP POLICY IF EXISTS org_isolation_read ON carts;

DROP FUNCTION IF EXISTS is_current_org_cart_shop_group(uuid);

DROP FUNCTION IF EXISTS is_current_org_cart(uuid);

ALTER TABLE
    cart_items NO FORCE ROW LEVEL SECURITY;

ALTER TABLE
    cart_shop_groups NO FORCE ROW LEVEL SECURITY;

ALTER TABLE
    carts NO FORCE ROW LEVEL SECURITY;

ALTER TABLE
    cart_items DISABLE ROW LEVEL SECURITY;

ALTER TABLE
    cart_shop_groups DISABLE ROW LEVEL SECURITY;

ALTER TABLE
    carts DISABLE ROW LEVEL SECURITY;

-- +goose StatementEnd
