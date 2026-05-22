-- name: CreateIdentity :one
INSERT INTO
    identities ("type")
VALUES
    ($1)
RETURNING
    *;

-- name: GetIdentity :one
SELECT
    id,
    "type",
    created_at
FROM
    identities
WHERE
    id = $1
LIMIT
    1;
