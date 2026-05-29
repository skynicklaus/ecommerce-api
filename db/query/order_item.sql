-- name: CreateOrderItem :one
INSERT INTO
    order_items (
        order_id,
        product_id,
        product_variant_id,
        warehouse_id,
        product_name,
        product_slug,
        variant_name,
        sku,
        variant_attributes,
        thumbnail_asset_key,
        quantity,
        unit_price,
        subtotal,
        discount_total,
        tax_total,
        total,
        currency
    )
VALUES
    (
        sqlc.arg('order_id')::UUID,
        sqlc.narg('product_id')::UUID,
        sqlc.narg('product_variant_id')::UUID,
        sqlc.narg('warehouse_id')::BIGINT,
        sqlc.arg('product_name')::TEXT,
        sqlc.narg('product_slug')::TEXT,
        sqlc.arg('variant_name')::TEXT,
        sqlc.arg('sku')::TEXT,
        sqlc.narg('variant_attributes')::JSONB,
        sqlc.narg('thumbnail_asset_key')::TEXT,
        sqlc.arg('quantity')::INT,
        sqlc.arg('unit_price')::NUMERIC,
        sqlc.arg('subtotal')::NUMERIC,
        sqlc.arg('discount_total')::NUMERIC,
        sqlc.arg('tax_total')::NUMERIC,
        sqlc.arg('total')::NUMERIC,
        sqlc.arg('currency')::TEXT
    )
RETURNING
    *;

-- name: GetOrderItemByID :one
SELECT
    *
FROM
    order_items
WHERE
    id = sqlc.arg('order_item_id')::UUID
LIMIT
    1;

-- name: ListOrderItemsByOrderID :many
SELECT
    *
FROM
    order_items
WHERE
    order_id = sqlc.arg('order_id')::UUID
ORDER BY
    created_at,
    id;

-- name: ListOrderItemsByCheckoutSession :many
SELECT
    oi.*
FROM
    order_items oi
    JOIN orders o ON o.id = oi.order_id
WHERE
    o.checkout_session_id = sqlc.arg('checkout_session_id')::UUID
ORDER BY
    o.merchant_org_id,
    oi.created_at,
    oi.id;

-- name: GetOrderItemWithOrderForReservation :one
SELECT
    oi.id,
    oi.order_id,
    oi.product_variant_id,
    oi.warehouse_id,
    oi.quantity,
    o.checkout_session_id,
    o.buyer_org_id,
    o.merchant_org_id
FROM
    order_items oi
    JOIN orders o ON o.id = oi.order_id
WHERE
    oi.id = sqlc.arg('order_item_id')::UUID
LIMIT
    1;
