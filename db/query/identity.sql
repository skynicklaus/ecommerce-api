-- name: CreateIdentity :one
INSERT INTO identities (
    type
) VALUES ($1)
RETURNING *;
