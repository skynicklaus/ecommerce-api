-- +goose Up
-- +goose StatementBegin
SET lock_timeout = '2s';
ALTER TABLE categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE products ENABLE ROW LEVEL SECURITY;
ALTER TABLE product_variants ENABLE ROW LEVEL SECURITY;
ALTER TABLE warehouses ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventories ENABLE ROW LEVEL SECURITY;

ALTER TABLE categories FORCE ROW LEVEL SECURITY;
ALTER TABLE products FORCE ROW LEVEL SECURITY;
ALTER TABLE product_variants FORCE ROW LEVEL SECURITY;
ALTER TABLE warehouses FORCE ROW LEVEL SECURITY;
ALTER TABLE inventories FORCE ROW LEVEL SECURITY;

CREATE OR REPLACE FUNCTION is_product_active(product_id uuid) RETURNS bool AS $$
    SELECT EXISTS (
        SELECT 1 FROM products
        WHERE id = product_id
        AND status = 'active'
    )
  $$ LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = pg_catalog, public;

CREATE OR REPLACE FUNCTION is_organization_warehouse(p_warehouse_id bigint)
RETURNS bool AS $$
    SELECT EXISTS (
        SELECT 1 FROM warehouses
        WHERE id = p_warehouse_id
        AND organization_id = current_org_id()
    )
  $$ LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = pg_catalog, public;

CREATE POLICY org_isolation_read ON categories
FOR SELECT
USING (
    is_active = TRUE
    OR organization_id = current_org_id()
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON categories
FOR INSERT
WITH CHECK (
    organization_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON categories
FOR UPDATE
USING (
    organization_id = current_org_id()
    OR is_platform_admin()
)
WITH CHECK (
    organization_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON categories
FOR DELETE
USING (
    organization_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON products
FOR SELECT
USING (
    status = 'active'
    OR organization_id = current_org_id()
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON products
FOR INSERT
WITH CHECK (
    organization_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON products
FOR UPDATE
USING (
    organization_id = current_org_id()
    OR is_platform_admin()
)
WITH CHECK (
    organization_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON products
FOR DELETE
USING (
    organization_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON product_variants
FOR SELECT
USING (
    is_product_active(product_id)
    OR organization_id = current_org_id()
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON product_variants
FOR INSERT
WITH CHECK (
    organization_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON product_variants
FOR UPDATE
USING (
    organization_id = current_org_id()
    OR is_platform_admin()
)
WITH CHECK (
    organization_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON product_variants
FOR DELETE
USING (
    organization_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON warehouses
FOR SELECT
USING (
    organization_id = current_org_id()
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON warehouses
FOR INSERT
WITH CHECK (
    organization_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON warehouses
FOR UPDATE
USING (
    organization_id = current_org_id()
    OR is_platform_admin()
)
WITH CHECK (
    organization_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON warehouses
FOR DELETE
USING (
    organization_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON inventories
FOR SELECT
USING (
    is_organization_warehouse(warehouse_id)
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON inventories
FOR INSERT
WITH CHECK (
    is_organization_warehouse(warehouse_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON inventories
FOR UPDATE
USING (
    is_organization_warehouse(warehouse_id)
    OR is_platform_admin()
)
WITH CHECK (
    is_organization_warehouse(warehouse_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON inventories
FOR DELETE
USING (
    is_organization_warehouse(warehouse_id)
    OR is_platform_admin()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP POLICY IF EXISTS org_isolation_delete ON inventories;
DROP POLICY IF EXISTS org_isolation_update ON inventories;
DROP POLICY IF EXISTS org_isolation_write ON inventories;
DROP POLICY IF EXISTS org_isolation_read ON inventories;
DROP POLICY IF EXISTS org_isolation_delete ON warehouses;
DROP POLICY IF EXISTS org_isolation_update ON warehouses;
DROP POLICY IF EXISTS org_isolation_write ON warehouses;
DROP POLICY IF EXISTS org_isolation_read ON warehouses;
DROP POLICY IF EXISTS org_isolation_delete ON product_variants;
DROP POLICY IF EXISTS org_isolation_update ON product_variants;
DROP POLICY IF EXISTS org_isolation_write ON product_variants;
DROP POLICY IF EXISTS org_isolation_read ON product_variants;
DROP POLICY IF EXISTS org_isolation_delete ON products;
DROP POLICY IF EXISTS org_isolation_update ON products;
DROP POLICY IF EXISTS org_isolation_write ON products;
DROP POLICY IF EXISTS org_isolation_read ON products;
DROP POLICY IF EXISTS org_isolation_delete ON categories;
DROP POLICY IF EXISTS org_isolation_update ON categories;
DROP POLICY IF EXISTS org_isolation_write ON categories;
DROP POLICY IF EXISTS org_isolation_read ON categories;
DROP FUNCTION IF EXISTS is_organization_warehouse(bigint);
DROP FUNCTION IF EXISTS is_product_active(uuid);
ALTER TABLE inventories NO FORCE ROW LEVEL SECURITY;
ALTER TABLE warehouses NO FORCE ROW LEVEL SECURITY;
ALTER TABLE product_variants NO FORCE ROW LEVEL SECURITY;
ALTER TABLE products NO FORCE ROW LEVEL SECURITY;
ALTER TABLE categories NO FORCE ROW LEVEL SECURITY;
ALTER TABLE inventories DISABLE ROW LEVEL SECURITY;
ALTER TABLE warehouses DISABLE ROW LEVEL SECURITY;
ALTER TABLE product_variants DISABLE ROW LEVEL SECURITY;
ALTER TABLE products DISABLE ROW LEVEL SECURITY;
ALTER TABLE categories DISABLE ROW LEVEL SECURITY;
-- +goose StatementEnd
