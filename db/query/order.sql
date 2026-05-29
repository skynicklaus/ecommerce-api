-- name: CreateOrder :one
INSERT INTO
    orders (
        checkout_session_id,
        merchant_org_id,
        buyer_customer_id,
        buyer_org_id,
        buyer_member_id,
        order_number,
        "status",
        payment_status,
        fulfillment_status,
        customer_email,
        customer_name,
        billing_address_snapshot,
        shipping_address_snapshot,
        subtotal,
        tax_total,
        shipping_total,
        shipping_discount,
        coupon_discount,
        grand_total,
        currency
    )
VALUES
    (
        sqlc.arg('checkout_session_id')::UUID,
        sqlc.arg('merchant_org_id')::UUID,
        sqlc.arg('buyer_customer_id')::UUID,
        sqlc.arg('buyer_org_id')::UUID,
        sqlc.arg('buyer_member_id')::UUID,
        sqlc.arg('order_number')::TEXT,
        COALESCE(sqlc.narg('status')::TEXT, 'pending_payment'),
        COALESCE(sqlc.narg('payment_status')::TEXT, 'unpaid'),
        COALESCE(sqlc.narg('fulfillment_status')::TEXT, 'unfulfilled'),
        sqlc.arg('customer_email')::TEXT,
        sqlc.arg('customer_name')::TEXT,
        sqlc.narg('billing_address_snapshot')::JSONB,
        sqlc.arg('shipping_address_snapshot')::JSONB,
        sqlc.arg('subtotal')::NUMERIC,
        sqlc.arg('tax_total')::NUMERIC,
        sqlc.arg('shipping_total')::NUMERIC,
        sqlc.arg('shipping_discount')::NUMERIC,
        sqlc.arg('coupon_discount')::NUMERIC,
        sqlc.arg('grand_total')::NUMERIC,
        sqlc.arg('currency')::TEXT
    )
RETURNING
    *;

-- name: GetOrderByID :one
SELECT
    *
FROM
    orders
WHERE
    id = sqlc.arg('order_id')::UUID
LIMIT
    1;

-- name: GetOrderForBuyer :one
SELECT
    *
FROM
    orders
WHERE
    id = sqlc.arg('order_id')::UUID
    AND buyer_org_id = sqlc.arg('buyer_org_id')::UUID
LIMIT
    1;

-- name: GetOrderForMerchant :one
SELECT
    *
FROM
    orders
WHERE
    id = sqlc.arg('order_id')::UUID
    AND merchant_org_id = sqlc.arg('merchant_org_id')::UUID
LIMIT
    1;

-- name: GetOrderForUpdate :one
SELECT
    *
FROM
    orders
WHERE
    id = sqlc.arg('order_id')::UUID
FOR UPDATE;

-- name: ListOrdersByCheckoutSession :many
SELECT
    *
FROM
    orders
WHERE
    checkout_session_id = sqlc.arg('checkout_session_id')::UUID
ORDER BY
    merchant_org_id,
    id;

-- name: ListBuyerOrders :many
SELECT
    *
FROM
    orders
WHERE
    buyer_org_id = sqlc.arg('buyer_org_id')::UUID
    AND (
        NOT sqlc.arg('has_cursor')::BOOL
        OR (created_at, id) < (sqlc.arg('after_created_at')::TIMESTAMPTZ, sqlc.arg('after_id')::UUID)
    )
ORDER BY
    created_at DESC,
    id DESC
LIMIT
    sqlc.arg('page_limit')::INT;

-- name: ListMerchantOrders :many
SELECT
    *
FROM
    orders
WHERE
    merchant_org_id = sqlc.arg('merchant_org_id')::UUID
    AND (
        NOT sqlc.arg('has_status')::BOOL
        OR "status" = sqlc.arg('status')::TEXT
    )
    AND (
        NOT sqlc.arg('has_cursor')::BOOL
        OR (created_at, id) < (sqlc.arg('after_created_at')::TIMESTAMPTZ, sqlc.arg('after_id')::UUID)
    )
ORDER BY
    created_at DESC,
    id DESC
LIMIT
    sqlc.arg('page_limit')::INT;

-- name: MarkOrderPlacedPaid :one
UPDATE
    orders
SET
    "status" = 'placed',
    payment_status = 'paid',
    placed_at = COALESCE(placed_at, NOW()),
    paid_at = COALESCE(paid_at, NOW()),
    updated_at = NOW()
WHERE
    id = sqlc.arg('order_id')::UUID
    AND "status" = 'pending_payment'
RETURNING
    *;

-- name: MarkOrdersPlacedPaidByCheckoutSession :many
UPDATE
    orders
SET
    "status" = 'placed',
    payment_status = 'paid',
    placed_at = COALESCE(placed_at, NOW()),
    paid_at = COALESCE(paid_at, NOW()),
    updated_at = NOW()
WHERE
    checkout_session_id = sqlc.arg('checkout_session_id')::UUID
    AND "status" = 'pending_payment'
RETURNING
    *;

-- name: MarkOrderPaymentFailed :one
UPDATE
    orders
SET
    payment_status = 'failed',
    updated_at = NOW()
WHERE
    id = sqlc.arg('order_id')::UUID
    AND payment_status IN ('unpaid', 'authorized')
RETURNING
    *;

-- name: CancelOrder :one
UPDATE
    orders
SET
    "status" = 'cancelled',
    cancelled_at = COALESCE(cancelled_at, NOW()),
    updated_at = NOW()
WHERE
    id = sqlc.arg('order_id')::UUID
    AND "status" IN ('pending_payment', 'placed', 'processing')
RETURNING
    *;

-- name: ExpirePendingOrder :one
UPDATE
    orders
SET
    "status" = 'expired',
    updated_at = NOW()
WHERE
    id = sqlc.arg('order_id')::UUID
    AND "status" = 'pending_payment'
RETURNING
    *;

-- name: UpdateOrderFulfillmentStatus :one
UPDATE
    orders
SET
    fulfillment_status = sqlc.arg('fulfillment_status')::TEXT,
    delivered_at = CASE
        WHEN sqlc.arg('fulfillment_status')::TEXT = 'delivered' THEN COALESCE(delivered_at, NOW())
        ELSE delivered_at
    END,
    completed_at = CASE
        WHEN sqlc.arg('fulfillment_status')::TEXT = 'delivered' THEN COALESCE(completed_at, NOW())
        ELSE completed_at
    END,
    updated_at = NOW()
WHERE
    id = sqlc.arg('order_id')::UUID
RETURNING
    *;
