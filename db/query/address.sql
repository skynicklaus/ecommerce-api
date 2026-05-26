-- name: CreateAddress :one
INSERT INTO
    addresses (
        organization_id,
        "type",
        label,
        line1,
        line2,
        postal_code,
        city,
        state,
        country
    )
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING
    *;

-- name: GetAddressByID :one
SELECT
    id,
    organization_id,
    "type",
    label,
    line1,
    line2,
    postal_code,
    city,
    state,
    country,
    is_default_shipping,
    is_default_billing,
    created_at
FROM
    addresses
WHERE
    id = $1
ORDER BY
    id
LIMIT
    1;

-- name: UpdateAddressByIDAndOrganization :one
UPDATE
    addresses
SET
    label = $3,
    line1 = $4,
    line2 = $5,
    postal_code = $6,
    city = $7,
    state = $8,
    country = $9,
    updated_at = NOW()
WHERE
    id = $1
    AND organization_id = $2
RETURNING
    *;
