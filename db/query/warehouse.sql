-- name: CreateWarehouse :one
INSERT INTO
    warehouses (
        organization_id,
        name,
        address_id
    )
VALUES
    ($1, $2, $3)
RETURNING
    *;

-- name: ListWarehousesByOrganization :many
SELECT
    w.id,
    w.organization_id,
    w.name,
    w.address_id,
    w.is_active,
    a.label AS address_label,
    a.line1 AS address_line1,
    a.line2 AS address_line2,
    a.postal_code AS address_postal_code,
    a.city AS address_city,
    a.state AS address_state,
    a.country AS address_country
FROM
    warehouses w
    JOIN addresses a ON a.id = w.address_id
WHERE
    w.organization_id = $1
ORDER BY
    w.id;

-- name: GetWarehouseByIDAndOrganization :one
SELECT
    w.id,
    w.organization_id,
    w.name,
    w.address_id,
    w.is_active,
    a.label AS address_label,
    a.line1 AS address_line1,
    a.line2 AS address_line2,
    a.postal_code AS address_postal_code,
    a.city AS address_city,
    a.state AS address_state,
    a.country AS address_country
FROM
    warehouses w
    JOIN addresses a ON a.id = w.address_id
WHERE
    w.id = $1
    AND w.organization_id = $2
LIMIT
    1;

-- name: UpdateWarehouse :one
UPDATE
    warehouses
SET
    name = $3,
    is_active = $4
WHERE
    id = $1
    AND organization_id = $2
RETURNING
    *;
