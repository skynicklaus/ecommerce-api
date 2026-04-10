-- name: CreateProduct :one
INSERT INTO products (
    organization_id
    , category_id
    , name
    , slug
    , description
    , specification
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetProductByID :one
SELECT
    id
    , organization_id
    , category_id
    , name
    , slug
    , description
    , status
    , is_featured
    , specification
    , created_at
    , updated_at
FROM products
WHERE id = $1
ORDER BY id
LIMIT 1;
