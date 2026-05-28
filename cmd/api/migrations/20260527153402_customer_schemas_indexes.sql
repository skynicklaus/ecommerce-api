-- +goose NO TRANSACTION
-- +goose Up
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_carts_customer_org_id ON carts(customer_org_id);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_cart_groups_cart_org_id ON cart_shop_groups(cart_id, merchant_org_id);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_cart_item_group_variant_id ON cart_items(cart_shop_group_id, product_variant_id);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS uq_cart_item_group_variant_id;

DROP INDEX CONCURRENTLY IF EXISTS uq_cart_groups_cart_org_id;

DROP INDEX CONCURRENTLY IF EXISTS uq_carts_customer_org_id;
