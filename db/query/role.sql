-- name: CreateRole :one
INSERT INTO roles (
    role_name
    , organization_id
    , organization_type
    , slug
    , is_system
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetRoleByID :one
SELECT
    id
    , role_name
    , organization_id
    , organization_type
    , slug
    , is_system
    , created_at
    , updated_at
FROM roles
WHERE id = $1
ORDER BY id
LIMIT 1;

-- name: ListOrganizationRolesByType :many
SELECT
    id
    , role_name
    , organization_id
    , organization_type
    , slug
    , is_system
    , created_at
    , updated_at
FROM roles
WHERE
    organization_id IS null
    OR organization_id = $1
    AND organization_type = $2
ORDER BY id;
