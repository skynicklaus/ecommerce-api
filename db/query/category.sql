-- name: CreateCategory :one
INSERT INTO
    categories (
        organization_id,
        parent_id,
        name,
        slug,
        description,
        sort_order
    )
VALUES
    ($1, $2, $3, $4, $5, $6)
RETURNING
    *;

-- name: GetCategoryByID :one
SELECT
    id,
    organization_id,
    parent_id,
    name,
    slug,
    description,
    sort_order,
    is_active,
    created_at
FROM
    categories
WHERE
    id = $1
ORDER BY
    id
LIMIT
    1;

-- name: ListCategoryPath :many
WITH RECURSIVE ancestors AS (
    SELECT
        c.id,
        c.organization_id,
        c.parent_id,
        c.name,
        c.slug,
        c.description,
        c.sort_order,
        c.is_active,
        c.created_at,
        0 AS depth
    FROM
        categories c
    WHERE
        c.id = $1
    UNION ALL
    SELECT
        c.id,
        c.organization_id,
        c.parent_id,
        c.name,
        c.slug,
        c.description,
        c.sort_order,
        c.is_active,
        c.created_at,
        a.depth + 1 AS depth
    FROM
        categories c
        JOIN ancestors a ON a.parent_id = c.id
)
SELECT
    id,
    organization_id,
    parent_id,
    name,
    slug,
    description,
    sort_order,
    is_active,
    created_at
FROM
    ancestors
ORDER BY
    depth DESC;
