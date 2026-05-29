-- +goose NO TRANSACTION
-- +goose Up
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_carts_buyer_org_id ON carts(buyer_org_id);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_cart_groups_cart_org_id ON cart_shop_groups(cart_id, merchant_org_id);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_cart_item_group_variant_id ON cart_items(cart_shop_group_id, product_variant_id);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_checkout_sessions_buyer_member_idem_key ON checkout_sessions(buyer_org_id, buyer_member_id, idempotency_key)
WHERE
    idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_checkout_sessions_one_active_per_buyer ON checkout_sessions(buyer_org_id, buyer_member_id)
WHERE
    "status" IN ('pending', 'reserved', 'payment_pending');

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkout_sessions_buyer_created_id ON checkout_sessions(buyer_org_id, created_at DESC, id DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_checkout_sessions_status_expires ON checkout_sessions("status", expires_at)
WHERE
    "status" IN ('pending', 'reserved', 'payment_pending');

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_orders_merchant_order_number ON orders(merchant_org_id, order_number);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_checkout_session_id ON orders(checkout_session_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_buyer_created_id ON orders(buyer_org_id, created_at DESC, id DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_buyer_member_created_id ON orders(
    buyer_org_id,
    buyer_member_id,
    created_at DESC,
    id DESC
);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_merchant_created_id ON orders(merchant_org_id, created_at DESC, id DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_status_created ON orders("status", created_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_payment_status_created ON orders(payment_status, created_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_fulfillment_status_created ON orders(fulfillment_status, created_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_order_items_order_id ON order_items(order_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_order_items_product_variant_id ON order_items(product_variant_id)
WHERE
    product_variant_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_order_items_product_id ON order_items(product_id)
WHERE
    product_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_inventory_reservations_checkout_session_id ON inventory_reservations(checkout_session_id);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_active_inventory_reservation_order ON inventory_reservations(order_id)
WHERE
    "status" = 'active';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_inventory_reservations_order_id ON inventory_reservations(order_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_inventory_reservations_buyer_created ON inventory_reservations(buyer_org_id, created_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_inventory_reservations_merchant_created ON inventory_reservations(merchant_org_id, created_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_inventory_reservations_active_expiry ON inventory_reservations(expires_at)
WHERE
    "status" = 'active';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_inventory_reservation_items_reservation_id ON inventory_reservation_items(reservation_id);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_inventory_reservation_items_reservation_order_item_warehouse ON inventory_reservation_items(reservation_id, order_item_id, warehouse_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_inventory_reservation_items_variant_warehouse ON inventory_reservation_items(product_variant_id, warehouse_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_inventory_reservation_items_order_item_id ON inventory_reservation_items(order_item_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_order_status_histories_order_created ON order_status_histories(order_id, created_at DESC);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_payments_provider_payment_id ON payments(provider, provider_payment_id)
WHERE
    provider_payment_id IS NOT NULL;

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_payments_checkout_session_id ON payments(checkout_session_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_payments_buyer_created ON payments(buyer_org_id, created_at DESC);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_payment_transactions_provider_ref ON payment_transactions(provider, provider_ref)
WHERE
    provider_ref IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_payment_transactions_payment_created ON payment_transactions(payment_id, created_at DESC);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_payment_transactions_payment_created;

DROP INDEX CONCURRENTLY IF EXISTS uq_payment_transactions_provider_ref;

DROP INDEX CONCURRENTLY IF EXISTS idx_payments_buyer_created;

DROP INDEX CONCURRENTLY IF EXISTS uq_payments_checkout_session_id;

DROP INDEX CONCURRENTLY IF EXISTS uq_payments_provider_payment_id;

DROP INDEX CONCURRENTLY IF EXISTS idx_order_status_histories_order_created;

DROP INDEX CONCURRENTLY IF EXISTS idx_inventory_reservation_items_order_item_id;

DROP INDEX CONCURRENTLY IF EXISTS idx_inventory_reservation_items_variant_warehouse;

DROP INDEX CONCURRENTLY IF EXISTS uq_inventory_reservation_items_reservation_order_item_warehouse;

DROP INDEX CONCURRENTLY IF EXISTS idx_inventory_reservation_items_reservation_id;

DROP INDEX CONCURRENTLY IF EXISTS idx_inventory_reservations_active_expiry;

DROP INDEX CONCURRENTLY IF EXISTS idx_inventory_reservations_merchant_created;

DROP INDEX CONCURRENTLY IF EXISTS idx_inventory_reservations_buyer_created;

DROP INDEX CONCURRENTLY IF EXISTS idx_inventory_reservations_order_id;

DROP INDEX CONCURRENTLY IF EXISTS uq_active_inventory_reservation_order;

DROP INDEX CONCURRENTLY IF EXISTS idx_inventory_reservations_checkout_session_id;

DROP INDEX CONCURRENTLY IF EXISTS idx_order_items_product_id;

DROP INDEX CONCURRENTLY IF EXISTS idx_order_items_product_variant_id;

DROP INDEX CONCURRENTLY IF EXISTS idx_order_items_order_id;

DROP INDEX CONCURRENTLY IF EXISTS idx_orders_fulfillment_status_created;

DROP INDEX CONCURRENTLY IF EXISTS idx_orders_payment_status_created;

DROP INDEX CONCURRENTLY IF EXISTS idx_orders_status_created;

DROP INDEX CONCURRENTLY IF EXISTS idx_orders_merchant_created_id;

DROP INDEX CONCURRENTLY IF EXISTS idx_orders_buyer_member_created_id;

DROP INDEX CONCURRENTLY IF EXISTS idx_orders_buyer_created_id;

DROP INDEX CONCURRENTLY IF EXISTS idx_orders_checkout_session_id;

DROP INDEX CONCURRENTLY IF EXISTS uq_orders_merchant_order_number;

DROP INDEX CONCURRENTLY IF EXISTS idx_checkout_sessions_status_expires;

DROP INDEX CONCURRENTLY IF EXISTS idx_checkout_sessions_buyer_created_id;

DROP INDEX CONCURRENTLY IF EXISTS uq_checkout_sessions_one_active_per_buyer;

DROP INDEX CONCURRENTLY IF EXISTS uq_checkout_sessions_buyer_member_idem_key;

DROP INDEX CONCURRENTLY IF EXISTS uq_cart_item_group_variant_id;

DROP INDEX CONCURRENTLY IF EXISTS uq_cart_groups_cart_org_id;

DROP INDEX CONCURRENTLY IF EXISTS uq_carts_buyer_org_id;
