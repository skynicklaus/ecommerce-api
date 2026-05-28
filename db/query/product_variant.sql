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

-- name: GetActiveVariantForCart :one
SELECT
    v.id,
    v.organization_id AS merchant_org_id,
    v.price,
    v.name AS variant_name,
    v.sku,
    v.is_active AS variant_is_active,
    p.id AS product_id,
    p.name AS product_name,
    p.slug AS product_slug,
    p.status AS product_status
FROM
    product_variants v
    JOIN products p ON p.id = v.product_id
WHERE
    v.id = $1
    AND v.is_active = TRUE
    AND p.status = 'active';

-- name: UpdateProductVariant :one
UPDATE
    product_variants
SET
    name = $3,
    price = $4,
    updated_at = NOW()
WHERE
    id = $1
    AND organization_id = $2
RETURNING
    *;

-- name: DeleteProductVariant :exec
UPDATE
    product_variants
SET
    is_active = FALSE,
    updated_at = NOW()
WHERE
    id = $1
    AND organization_id = $2;
