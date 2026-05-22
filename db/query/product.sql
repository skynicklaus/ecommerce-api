-- name: CreateProduct :one
INSERT INTO
    products (
        organization_id,
        category_id,
        name,
        slug,
        description,
        specification,
        idempotency_key
    )
VALUES
    ($1, $2, $3, $4, $5, $6, $7)
RETURNING
    *;

-- name: GetProductByIdempotencyKey :one
SELECT
    *
FROM
    products
WHERE
    organization_id = $1
    AND idempotency_key = $2;

-- name: UpdateProductStatus :one
UPDATE
    products
SET
    "status" = $3,
    updated_at = NOW()
WHERE
    id = $1
    AND organization_id = $2
RETURNING
    id,
    "status",
    updated_at;

-- name: GetProductByID :one
SELECT
    id,
    organization_id,
    category_id,
    name,
    slug,
    description,
    "status",
    is_featured,
    specification,
    created_at,
    updated_at
FROM
    products
WHERE
    id = $1
ORDER BY
    id
LIMIT
    1;

-- name: GetProductBySlug :one
SELECT
    id,
    organization_id,
    category_id,
    name,
    slug,
    description,
    "status",
    is_featured,
    specification,
    created_at,
    updated_at
FROM
    products
WHERE
    slug = $1
ORDER BY
    id
LIMIT
    1;

-- name: ListProductsByOrganization :many
SELECT
    id,
    organization_id,
    category_id,
    name,
    slug,
    description,
    "status",
    is_featured,
    specification,
    created_at,
    updated_at
FROM
    products
WHERE
    organization_id = $1
ORDER BY
    created_at DESC;

-- name: GetActiveProductByID :one
SELECT
    id,
    organization_id,
    category_id,
    name,
    slug,
    description,
    "status",
    is_featured,
    specification,
    created_at,
    updated_at
FROM
    products
WHERE
    id = $1
    AND "status" = 'active'
LIMIT
    1;

-- name: GetActiveProductBySlug :one
SELECT
    id,
    organization_id,
    category_id,
    name,
    slug,
    description,
    "status",
    is_featured,
    specification,
    created_at,
    updated_at
FROM
    products
WHERE
    slug = $1
    AND "status" = 'active'
LIMIT
    1;

-- name: ListActiveProductsAfter :many
SELECT
    id,
    organization_id,
    category_id,
    name,
    slug,
    description,
    "status",
    is_featured,
    specification,
    created_at,
    updated_at
FROM
    products
WHERE
    "status" = 'active'
    AND (created_at, id) < (
        sqlc.arg ('after_created_at')::TIMESTAMPTZ,
        sqlc.arg ('after_id')::UUID
    )
ORDER BY
    created_at DESC,
    id DESC
LIMIT
    sqlc.arg ('page_limit')::INT;
