-- name: CreatePayment :one
INSERT INTO
    payments (
        checkout_session_id,
        buyer_org_id,
        provider,
        provider_payment_id,
        "status",
        amount,
        currency,
        metadata
    )
VALUES
    (
        sqlc.arg('checkout_session_id')::UUID,
        sqlc.arg('buyer_org_id')::UUID,
        sqlc.arg('provider')::TEXT,
        sqlc.narg('provider_payment_id')::TEXT,
        COALESCE(sqlc.narg('status')::TEXT, 'pending'),
        sqlc.arg('amount')::NUMERIC,
        sqlc.arg('currency')::TEXT,
        sqlc.narg('metadata')::JSONB
    )
RETURNING
    *;

-- name: GetPaymentByID :one
SELECT
    *
FROM
    payments
WHERE
    id = sqlc.arg('payment_id')::UUID
LIMIT
    1;

-- name: GetPaymentForUpdate :one
SELECT
    *
FROM
    payments
WHERE
    id = sqlc.arg('payment_id')::UUID
FOR UPDATE;

-- name: GetPaymentByProviderPaymentID :one
SELECT
    *
FROM
    payments
WHERE
    provider = sqlc.arg('provider')::TEXT
    AND provider_payment_id = sqlc.arg('provider_payment_id')::TEXT
LIMIT
    1;

-- name: ListPaymentsByCheckoutSession :many
SELECT
    *
FROM
    payments
WHERE
    checkout_session_id = sqlc.arg('checkout_session_id')::UUID
ORDER BY
    created_at,
    id;

-- name: UpdatePaymentProviderPaymentID :one
UPDATE
    payments
SET
    provider_payment_id = sqlc.arg('provider_payment_id')::TEXT,
    updated_at = NOW()
WHERE
    id = sqlc.arg('payment_id')::UUID
RETURNING
    *;

-- name: UpdatePaymentStatus :one
UPDATE
    payments
SET
    "status" = sqlc.arg('status')::TEXT,
    metadata = COALESCE(sqlc.narg('metadata')::JSONB, metadata),
    updated_at = NOW()
WHERE
    id = sqlc.arg('payment_id')::UUID
RETURNING
    *;

-- name: MarkPaymentSucceeded :one
UPDATE
    payments
SET
    "status" = 'succeeded',
    metadata = COALESCE(sqlc.narg('metadata')::JSONB, metadata),
    updated_at = NOW()
WHERE
    id = sqlc.arg('payment_id')::UUID
    AND "status" IN ('pending', 'requires_action', 'authorized')
RETURNING
    *;

-- name: MarkPaymentFailed :one
UPDATE
    payments
SET
    "status" = 'failed',
    metadata = COALESCE(sqlc.narg('metadata')::JSONB, metadata),
    updated_at = NOW()
WHERE
    id = sqlc.arg('payment_id')::UUID
    AND "status" IN ('pending', 'requires_action', 'authorized')
RETURNING
    *;

-- name: CreatePaymentTransaction :one
INSERT INTO
    payment_transactions (
        payment_id,
        "type",
        "status",
        provider,
        provider_ref,
        amount,
        currency,
        failure_code,
        failure_message,
        metadata,
        processed_at
    )
VALUES
    (
        sqlc.arg('payment_id')::UUID,
        sqlc.arg('type')::TEXT,
        sqlc.arg('status')::TEXT,
        sqlc.arg('provider')::TEXT,
        sqlc.narg('provider_ref')::TEXT,
        sqlc.arg('amount')::NUMERIC,
        sqlc.arg('currency')::TEXT,
        sqlc.narg('failure_code')::TEXT,
        sqlc.narg('failure_message')::TEXT,
        sqlc.narg('metadata')::JSONB,
        sqlc.narg('processed_at')::TIMESTAMPTZ
    )
RETURNING
    *;

-- name: GetPaymentTransactionByProviderRef :one
SELECT
    *
FROM
    payment_transactions
WHERE
    provider = sqlc.arg('provider')::TEXT
    AND provider_ref = sqlc.arg('provider_ref')::TEXT
LIMIT
    1;

-- name: ListPaymentTransactionsByPayment :many
SELECT
    *
FROM
    payment_transactions
WHERE
    payment_id = sqlc.arg('payment_id')::UUID
ORDER BY
    created_at,
    id;
