-- name: CreateCustomer :one
INSERT INTO
    customers (identity_id, name, email)
VALUES
    ($1, $2, $3)
RETURNING
    *;

-- name: GetCustomerByEmail :one
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
    customers
WHERE
    email = $1
ORDER BY
    id
LIMIT
    1;

-- name: GetCustomerByIdentityID :one
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
    customers
WHERE
    identity_id = $1
LIMIT
    1;

-- name: GetCustomerWithCredential :one
SELECT
    c.id,
    c.identity_id,
    c.name,
    c.email,
    ca.hashed_password
FROM
    customers c
    LEFT JOIN customer_accounts ca ON c.id = ca.customer_id
    AND ca.provider_id = 'credential'
WHERE
    c.email = $1
LIMIT
    1;
