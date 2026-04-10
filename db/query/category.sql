-- name: CreateCategory :one
INSERT INTO categories (
    organization_id
    , parent_id
    , name
    , slug
    , description
    , sort_order
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetCategoryByID :one
SELECT
    id
    , organization_id
    , parent_id
    , name
    , slug
    , description
    , sort_order
    , is_active
    , created_at
FROM categories
WHERE id = $1
ORDER BY id
LIMIT 1;
