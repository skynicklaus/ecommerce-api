-- name: CreateUser :one
INSERT INTO users (
    identity_id
    , name
    , email
) VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUserByEmail :one
SELECT
    id
    , identity_id
    , name
    , email
    , email_verified
    , image
    , created_at
    , updated_at
FROM users
WHERE email = $1
ORDER BY id
LIMIT 1;
