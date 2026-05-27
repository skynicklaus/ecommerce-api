-- name: CreateOrganization :one
INSERT INTO
    organizations (
        parent_id,
        name,
        slug,
        "status",
        "type",
        capability,
        metadata
    )
VALUES
    ($1, $2, $3, $4, $5, $6, $7)
RETURNING
    *;

-- name: GetOrganizationByID :one
SELECT
    *
FROM
    organizations
WHERE
    id = $1
ORDER BY
    id
LIMIT
    1;

-- name: GetOrganizationBySlug :one
SELECT
    *
FROM
    organizations
WHERE
    slug = $1
ORDER BY
    id
LIMIT
    1;
