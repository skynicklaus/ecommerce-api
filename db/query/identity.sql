-- name: CreateIdentity :one
INSERT INTO
    identities (TYPE)
VALUES
    ($1)
RETURNING
    *;
