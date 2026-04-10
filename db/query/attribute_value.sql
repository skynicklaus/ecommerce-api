-- name: CreateAttributeValue :one
INSERT INTO attribute_values (
    attribute_id
    , organization_id
    , value
    , label
    , sort_order
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;
