-- name: CreateMember :one
INSERT INTO members (
    identity_id
    , organization_id
) VALUES ($1, $2)
RETURNING *;
