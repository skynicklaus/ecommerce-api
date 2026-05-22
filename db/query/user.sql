-- name: CreateUser :one
INSERT INTO
    users (identity_id, name, email)
VALUES
    ($1, $2, $3)
RETURNING
    *;

-- name: GetUserByEmail :one
SELECT
    id,
    identity_id,
    name,
    email,
    email_verified,
    image,
    created_at,
    updated_at
FROM
    users
WHERE
    email = $1
ORDER BY
    id
LIMIT
    1;

-- name: GetUserByIdentityID :one
SELECT
    id,
    identity_id,
    name,
    email,
    email_verified,
    image,
    created_at,
    updated_at
FROM
    users
WHERE
    identity_id = $1
LIMIT
    1;

-- name: GetUserWithCredential :one
SELECT
    u.id,
    u.identity_id,
    u.name,
    u.email,
    ua.hashed_password
FROM
    users u
LEFT JOIN user_accounts ua
    ON u.id = ua.user_id AND ua.provider_id = 'credential'
WHERE
    u.email = $1
LIMIT 1;


