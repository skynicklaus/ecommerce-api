-- name: CreateWarehouse :one
INSERT INTO warehouses (
    organization_id
    , name
    , address_id
) VALUES ($1, $2, $3)
RETURNING *;
