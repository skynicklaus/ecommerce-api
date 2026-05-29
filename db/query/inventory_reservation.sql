-- name: CreateInventoryReservation :one
INSERT INTO
    inventory_reservations (
        checkout_session_id,
        order_id,
        buyer_org_id,
        merchant_org_id,
        expires_at
    )
VALUES
    (
        sqlc.arg('checkout_session_id')::UUID,
        sqlc.arg('order_id')::UUID,
        sqlc.arg('buyer_org_id')::UUID,
        sqlc.arg('merchant_org_id')::UUID,
        sqlc.arg('expires_at')::TIMESTAMPTZ
    )
RETURNING
    *;

-- name: GetInventoryReservationByID :one
SELECT
    *
FROM
    inventory_reservations
WHERE
    id = sqlc.arg('reservation_id')::UUID
LIMIT
    1;

-- name: GetInventoryReservationForUpdate :one
SELECT
    *
FROM
    inventory_reservations
WHERE
    id = sqlc.arg('reservation_id')::UUID
FOR UPDATE;

-- name: GetActiveInventoryReservationByOrderForUpdate :one
SELECT
    *
FROM
    inventory_reservations
WHERE
    order_id = sqlc.arg('order_id')::UUID
    AND "status" = 'active'
FOR UPDATE;

-- name: ListInventoryReservationsByCheckoutSession :many
SELECT
    *
FROM
    inventory_reservations
WHERE
    checkout_session_id = sqlc.arg('checkout_session_id')::UUID
ORDER BY
    created_at,
    id;

-- name: ListExpiredActiveInventoryReservationsForUpdate :many
SELECT
    *
FROM
    inventory_reservations
WHERE
    "status" = 'active'
    AND expires_at <= NOW()
ORDER BY
    expires_at,
    id
LIMIT
    sqlc.arg('page_limit')::INT
FOR UPDATE SKIP LOCKED;

-- name: ConfirmInventoryReservation :one
UPDATE
    inventory_reservations
SET
    "status" = 'confirmed',
    confirmed_at = COALESCE(confirmed_at, NOW())
WHERE
    id = sqlc.arg('reservation_id')::UUID
    AND "status" = 'active'
RETURNING
    *;

-- name: ReleaseInventoryReservation :one
UPDATE
    inventory_reservations
SET
    "status" = 'released',
    released_at = COALESCE(released_at, NOW())
WHERE
    id = sqlc.arg('reservation_id')::UUID
    AND "status" = 'active'
RETURNING
    *;

-- name: ExpireInventoryReservation :one
UPDATE
    inventory_reservations
SET
    "status" = 'expired',
    released_at = COALESCE(released_at, NOW())
WHERE
    id = sqlc.arg('reservation_id')::UUID
    AND "status" = 'active'
RETURNING
    *;

-- name: CancelInventoryReservation :one
UPDATE
    inventory_reservations
SET
    "status" = 'cancelled',
    released_at = COALESCE(released_at, NOW())
WHERE
    id = sqlc.arg('reservation_id')::UUID
    AND "status" = 'active'
RETURNING
    *;

-- name: CreateInventoryReservationItem :one
INSERT INTO
    inventory_reservation_items (
        reservation_id,
        order_item_id,
        product_variant_id,
        warehouse_id,
        quantity
    )
VALUES
    (
        sqlc.arg('reservation_id')::UUID,
        sqlc.arg('order_item_id')::UUID,
        sqlc.arg('product_variant_id')::UUID,
        sqlc.arg('warehouse_id')::BIGINT,
        sqlc.arg('quantity')::INT
    )
RETURNING
    *;

-- name: ListInventoryReservationItems :many
SELECT
    *
FROM
    inventory_reservation_items
WHERE
    reservation_id = sqlc.arg('reservation_id')::UUID
ORDER BY
    created_at,
    id;

-- name: ListInventoryReservationItemsByCheckoutSession :many
SELECT
    iri.*
FROM
    inventory_reservation_items iri
    JOIN inventory_reservations ir ON ir.id = iri.reservation_id
WHERE
    ir.checkout_session_id = sqlc.arg('checkout_session_id')::UUID
ORDER BY
    ir.created_at,
    iri.created_at,
    iri.id;
