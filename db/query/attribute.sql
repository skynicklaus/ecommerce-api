-- name: CreateAttribute :one
INSERT INTO attributes (
    organization_id
    , name
    , slug
    , type
) VALUES ($1, $2, $3, $4)
RETURNING *;
