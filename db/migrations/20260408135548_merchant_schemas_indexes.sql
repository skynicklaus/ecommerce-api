-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_categories_organization_id ON categories (organization_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_categories_parent_id ON categories (parent_id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_categories_slug ON categories (slug, coalesce(organization_id, '00000000-0000-0000-0000-000000000000'::uuid));
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_products_organization_id ON products (organization_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_products_category_id ON products (category_id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_products_organization_slug ON products (organization_id, slug);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_product_variants_product_id ON product_variants (product_id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_product_variants_organization_sku ON product_variants (organization_id, sku);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_product_assets_primary ON product_assets (product_id) WHERE is_primary = TRUE;
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_product_assets_variant ON product_assets (product_variant_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_warehouses_organization_id ON warehouses (organization_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_warehouses_address_id ON warehouses (address_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_inventories_warehouse_id ON inventories (warehouse_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_inventories_low_stock ON inventories (warehouse_id, quantity_available) WHERE low_stock_threshold IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_attributes_organization_id ON attributes (organization_id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_attributes_slug ON attributes (slug, coalesce(organization_id, '00000000-0000-0000-0000-000000000000'::uuid));
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_attribute_values_attribute_id ON attribute_values (attribute_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_attribute_values_organization_id ON attribute_values (organization_id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_attribute_values_value ON attribute_values (attribute_id, lower(trim(value)));
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_product_variant_attributes_product_variant ON product_variant_attributes (product_variant_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_product_variant_attributes_attribute_value ON product_variant_attributes (attribute_value_id);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_product_variant_attributes_attribute_value;
DROP INDEX CONCURRENTLY IF EXISTS idx_product_variant_attributes_product_variant;
DROP INDEX CONCURRENTLY IF EXISTS uq_attribute_values_value;
DROP INDEX CONCURRENTLY IF EXISTS idx_attribute_values_organization_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_attribute_values_attribute_id;
DROP INDEX CONCURRENTLY IF EXISTS uq_attributes_slug;
DROP INDEX CONCURRENTLY IF EXISTS idx_attributes_organization_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_inventories_low_stock;
DROP INDEX CONCURRENTLY IF EXISTS idx_inventories_warehouse_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_warehouses_address_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_warehouses_organization_id;
DROP INDEX CONCURRENTLY IF EXISTS uq_product_assets_variant;
DROP INDEX CONCURRENTLY IF EXISTS uq_product_assets_primary;
DROP INDEX CONCURRENTLY IF EXISTS uq_product_variants_organization_sku;
DROP INDEX CONCURRENTLY IF EXISTS idx_product_variants_product_id;
DROP INDEX CONCURRENTLY IF EXISTS uq_products_organization_slug;
DROP INDEX CONCURRENTLY IF EXISTS idx_products_category_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_categories_organization_id;
DROP INDEX CONCURRENTLY IF EXISTS uq_categories_slug;
DROP INDEX CONCURRENTLY IF EXISTS idx_categories_parent_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_categories_organization_id;
