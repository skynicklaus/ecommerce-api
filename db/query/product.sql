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
    organization_id = $1
    AND slug = $2
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
    organization_id = $1
    AND slug = $2
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

-- name: SearchProducts :many
WITH search_query AS (
    SELECT
        websearch_to_tsquery('simple', sqlc.arg('query')) AS query
),
ranked_products AS (
    SELECT
        p.id,
        p.organization_id,
        p.category_id,
        p.name,
        p.slug,
        p.description,
        p."status",
        p.is_featured,
        p.specification,
        p.created_at,
        p.updated_at,
        ts_rank_cd(psd.search_vector, search_query.query)::DOUBLE PRECISION AS rank
    FROM
        products p
        JOIN product_search_documents psd ON psd.product_id = p.id,
        search_query
    WHERE
        p."status" = 'active'
        AND psd.search_vector @@ search_query.query
)
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
    updated_at,
    rank
FROM
    ranked_products
WHERE
    (rank, created_at, id) < (
        sqlc.arg('after_rank')::DOUBLE PRECISION,
        sqlc.arg('after_created_at')::TIMESTAMPTZ,
        sqlc.arg('after_id')::UUID
    )
ORDER BY
    rank DESC,
    created_at DESC,
    id DESC
LIMIT
    sqlc.arg('page_limit')::INT;

-- name: UpdateProduct :one
UPDATE
    products
SET
    category_id = $3,
    name = $4,
    slug = $5,
    description = $6,
    specification = $7,
    "status" = $8,
    is_featured = $9,
    updated_at = NOW()
WHERE
    id = $1
    AND organization_id = $2
RETURNING
    *;

-- name: DeleteProduct :exec
WITH archived_product AS (
    UPDATE
        products
    SET
        "status" = 'archived',
        updated_at = NOW()
    WHERE
        products.id = $1
        AND products.organization_id = $2
    RETURNING
        id,
        organization_id
)
UPDATE
    product_variants v
SET
    is_active = FALSE,
    updated_at = NOW()
FROM
    archived_product p
WHERE
    v.product_id = p.id
    AND v.organization_id = p.organization_id;

-- name: ListProductsByOrganizationWithStatus :many
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
    AND "status" = ANY(sqlc.arg('statuses')::TEXT [])
    AND (created_at, id) < (
        sqlc.arg ('after_created_at')::TIMESTAMPTZ,
        sqlc.arg ('after_id')::UUID
    )
ORDER BY
    created_at DESC,
    id DESC
LIMIT
    sqlc.arg ('page_limit')::INT;
