-- name: CreateOrganization :one
INSERT INTO organizations (
    parent_id
    , name
    , slug
    , status
    , type
    , metadata
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetOrganizationByID :one
SELECT
    id
    , parent_id
    , name
    , slug
    , status
    , type
    , logo
    , metadata
    , created_at
FROM organizations
WHERE id = $1
ORDER BY id
LIMIT 1;

-- name: GetOrganizationBySlug :one
SELECT
    id
    , parent_id
    , name
    , slug
    , status
    , type
    , logo
    , metadata
    , created_at
FROM organizations
WHERE slug = $1
ORDER BY id
LIMIT 1;
