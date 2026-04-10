-- name: CreateCustomerAccount :one
INSERT INTO customer_accounts (
    customer_id
    , account_id
    , provider_id
    , access_token
    , refresh_token
    , access_token_expires_at
    , refresh_token_expires_at
    , scope
    , id_token
    , hashed_password
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING
    id
    , customer_id
    , account_id
    , provider_id
    , access_token
    , refresh_token
    , access_token_expires_at
    , refresh_token_expires_at
    , scope
    , id_token
    , created_at
    , updated_at;

-- name: GetCustomerHashedPassword :one
SELECT hashed_password
FROM customer_accounts
WHERE customer_id = $1 AND provider_id = $2
ORDER BY id
LIMIT 1;

-- name: GetCustomerAccountByID :one
SELECT
    id
    , customer_id
    , account_id
    , provider_id
    , access_token
    , refresh_token
    , access_token_expires_at
    , refresh_token_expires_at
    , scope
    , id_token
    , created_at
    , updated_at
FROM customer_accounts
WHERE customer_id = $1
ORDER BY id
LIMIT 1;

-- name: UpdateCustomerAccount :one
UPDATE customer_accounts
SET
    access_token = coalesce($2, access_token)
    , refresh_token = coalesce($3, refresh_token)
    , access_token_expires_at = coalesce($4, access_token_expires_at)
    , refresh_token_expires_at = coalesce($5, refresh_token_expires_at)
    , id_token = coalesce($6, id_token)
    , scope = coalesce($7, scope)
    , hashed_password = coalesce($8, hashed_password)
WHERE id = $1
RETURNING
    id
    , customer_id
    , account_id
    , provider_id
    , access_token
    , refresh_token
    , access_token_expires_at
    , refresh_token_expires_at
    , scope
    , id_token
    , created_at
    , updated_at;
