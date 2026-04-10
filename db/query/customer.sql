-- name: CreateCustomer :one
INSERT INTO customers (
    identity_id
    , name
    , email
) VALUES ($1, $2, $3)
RETURNING *;

-- name: GetCustomerByEmail :one
SELECT
    id
    , identity_id
    , name
    , email
    , email_verified
    , image
    , created_at
    , updated_at
FROM customers
WHERE email = $1
ORDER BY id
LIMIT 1;
