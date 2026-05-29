-- name: CreateCheckoutSession :one
INSERT INTO
    checkout_sessions (
        buyer_customer_id,
        buyer_org_id,
        buyer_member_id,
        idempotency_key,
        checkout_fingerprint,
        subtotal,
        tax_total,
        shipping_total,
        discount_total,
        grand_total,
        currency,
        expires_at
    )
VALUES
    (
        sqlc.arg('buyer_customer_id')::UUID,
        sqlc.arg('buyer_org_id')::UUID,
        sqlc.arg('buyer_member_id')::UUID,
        sqlc.narg('idempotency_key')::TEXT,
        sqlc.arg('checkout_fingerprint')::TEXT,
        sqlc.arg('subtotal')::NUMERIC,
        sqlc.arg('tax_total')::NUMERIC,
        sqlc.arg('shipping_total')::NUMERIC,
        sqlc.arg('discount_total')::NUMERIC,
        sqlc.arg('grand_total')::NUMERIC,
        sqlc.arg('currency')::TEXT,
        sqlc.narg('expires_at')::TIMESTAMPTZ
    )
RETURNING
    *;

-- name: GetCheckoutSessionByID :one
SELECT
    *
FROM
    checkout_sessions
WHERE
    id = sqlc.arg('checkout_session_id')::UUID
LIMIT
    1;

-- name: GetCheckoutSessionForBuyer :one
SELECT
    *
FROM
    checkout_sessions
WHERE
    id = sqlc.arg('checkout_session_id')::UUID
    AND buyer_org_id = sqlc.arg('buyer_org_id')::UUID
LIMIT
    1;

-- name: GetCheckoutSessionForUpdate :one
SELECT
    *
FROM
    checkout_sessions
WHERE
    id = sqlc.arg('checkout_session_id')::UUID
FOR UPDATE;

-- name: GetCheckoutSessionByIdempotencyKey :one
SELECT
    *
FROM
    checkout_sessions
WHERE
    buyer_org_id = sqlc.arg('buyer_org_id')::UUID
    AND buyer_member_id = sqlc.arg('buyer_member_id')::UUID
    AND idempotency_key = sqlc.arg('idempotency_key')::TEXT
LIMIT
    1;

-- name: GetActiveCheckoutSessionForBuyer :one
SELECT
    *
FROM
    checkout_sessions
WHERE
    buyer_org_id = sqlc.arg('buyer_org_id')::UUID
    AND buyer_member_id = sqlc.arg('buyer_member_id')::UUID
    AND "status" IN ('pending', 'reserved', 'payment_pending')
ORDER BY
    created_at DESC,
    id DESC
LIMIT
    1;

-- name: GetActiveCheckoutSessionForBuyerForUpdate :one
SELECT
    *
FROM
    checkout_sessions
WHERE
    buyer_org_id = sqlc.arg('buyer_org_id')::UUID
    AND buyer_member_id = sqlc.arg('buyer_member_id')::UUID
    AND "status" IN ('pending', 'reserved', 'payment_pending')
ORDER BY
    created_at DESC,
    id DESC
LIMIT
    1
FOR UPDATE;

-- name: CancelActiveCheckoutSessionsForBuyer :many
UPDATE
    checkout_sessions
SET
    "status" = 'cancelled',
    cancelled_at = NOW(),
    updated_at = NOW()
WHERE
    buyer_org_id = sqlc.arg('buyer_org_id')::UUID
    AND buyer_member_id = sqlc.arg('buyer_member_id')::UUID
    AND "status" IN ('pending', 'reserved', 'payment_pending')
RETURNING
    *;

-- name: UpdateCheckoutSessionTotals :one
UPDATE
    checkout_sessions
SET
    subtotal = sqlc.arg('subtotal')::NUMERIC,
    tax_total = sqlc.arg('tax_total')::NUMERIC,
    shipping_total = sqlc.arg('shipping_total')::NUMERIC,
    discount_total = sqlc.arg('discount_total')::NUMERIC,
    grand_total = sqlc.arg('grand_total')::NUMERIC,
    updated_at = NOW()
WHERE
    id = sqlc.arg('checkout_session_id')::UUID
RETURNING
    *;

-- name: MarkCheckoutSessionReserved :one
UPDATE
    checkout_sessions
SET
    "status" = 'reserved',
    updated_at = NOW()
WHERE
    id = sqlc.arg('checkout_session_id')::UUID
    AND "status" = 'pending'
RETURNING
    *;

-- name: MarkCheckoutSessionPaymentPending :one
UPDATE
    checkout_sessions
SET
    "status" = 'payment_pending',
    updated_at = NOW()
WHERE
    id = sqlc.arg('checkout_session_id')::UUID
    AND "status" IN ('pending', 'reserved')
RETURNING
    *;

-- name: CompleteCheckoutSession :one
UPDATE
    checkout_sessions
SET
    "status" = 'completed',
    completed_at = COALESCE(completed_at, NOW()),
    updated_at = NOW()
WHERE
    id = sqlc.arg('checkout_session_id')::UUID
    AND "status" IN ('reserved', 'payment_pending')
RETURNING
    *;

-- name: CancelCheckoutSession :one
UPDATE
    checkout_sessions
SET
    "status" = 'cancelled',
    cancelled_at = COALESCE(cancelled_at, NOW()),
    updated_at = NOW()
WHERE
    id = sqlc.arg('checkout_session_id')::UUID
    AND "status" IN ('pending', 'reserved', 'payment_pending')
RETURNING
    *;

-- name: ExpireCheckoutSession :one
UPDATE
    checkout_sessions
SET
    "status" = 'expired',
    updated_at = NOW()
WHERE
    id = sqlc.arg('checkout_session_id')::UUID
    AND "status" IN ('pending', 'reserved', 'payment_pending')
RETURNING
    *;

-- name: ListExpiredActiveCheckoutSessionsForUpdate :many
SELECT
    *
FROM
    checkout_sessions
WHERE
    "status" IN ('pending', 'reserved', 'payment_pending')
    AND expires_at IS NOT NULL
    AND expires_at <= NOW()
ORDER BY
    expires_at,
    id
LIMIT
    sqlc.arg('page_limit')::INT
FOR UPDATE SKIP LOCKED;
