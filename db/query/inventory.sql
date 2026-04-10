-- name: CreateInventory :one
INSERT INTO inventories (
    product_variant_id
    , warehouse_id
    , quantity_on_hand
    , low_stock_threshold
) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetWarehouseVariantInventory :one
SELECT
    product_variant_id
    , warehouse_id
    , quantity_on_hand
    , quantity_reserved
    , quantity_available
    , low_stock_threshold
    , is_active
FROM inventories
WHERE product_variant_id = $1 AND warehouse_id = $2
ORDER BY product_variant_id
LIMIT 1;
