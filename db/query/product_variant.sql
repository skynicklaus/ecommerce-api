-- name: CreateProductVariant :one
INSERT INTO
    product_variants (
        product_id,
        organization_id,
        sku,
        name,
        price
    )
VALUES
    ($1, $2, $3, $4, $5)
RETURNING
    *;

-- name: GetProductVariantByID :one
SELECT
    id,
    product_id,
    organization_id,
    sku,
    name,
    price,
    track_inventory,
    is_active,
    created_at,
    updated_at
FROM
    product_variants
WHERE
    id = $1
ORDER BY
    id
LIMIT
    1;

-- name: ListProductVariantsByProductID :many
SELECT
    id,
    product_id,
    organization_id,
    sku,
    name,
    price,
    track_inventory,
    is_active,
    created_at,
    updated_at
FROM
    product_variants
WHERE
    product_id = $1
ORDER BY
    id;

-- name: ListProductVariantsByProductIDs :many
SELECT
    id,
    product_id,
    organization_id,
    sku,
    name,
    price,
    track_inventory,
    is_active,
    created_at,
    updated_at
FROM
    product_variants
WHERE
    product_id = ANY (sqlc.arg ('product_ids')::UUID [])
ORDER BY
    product_id,
    id;
