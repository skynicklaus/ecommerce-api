-- name: CreateInventory :one
INSERT INTO
    inventories (
        product_variant_id,
        warehouse_id,
        quantity_on_hand,
        low_stock_threshold
    )
VALUES
    ($1, $2, $3, $4)
RETURNING
    *;

-- name: GetWarehouseVariantInventory :one
SELECT
    i.product_variant_id,
    i.warehouse_id,
    i.quantity_on_hand,
    i.quantity_reserved,
    i.quantity_available,
    i.low_stock_threshold,
    i.is_active
FROM
    inventories i
    JOIN product_variants pv ON pv.id = i.product_variant_id
    JOIN warehouses w ON w.id = i.warehouse_id
WHERE
    pv.organization_id = sqlc.arg('organization_id')::UUID
    AND w.organization_id = sqlc.arg('organization_id')::UUID
    AND i.product_variant_id = sqlc.arg('product_variant_id')::UUID
    AND i.warehouse_id = sqlc.arg('warehouse_id')::BIGINT
ORDER BY
    i.product_variant_id
LIMIT
    1;

-- name: UpsertInventory :one
WITH variant AS (
    SELECT
        id
    FROM
        product_variants
    WHERE
        id = sqlc.arg('product_variant_id')::UUID
        AND organization_id = sqlc.arg('organization_id')::UUID
),
warehouse AS (
    SELECT
        id
    FROM
        warehouses
    WHERE
        id = sqlc.arg('warehouse_id')::BIGINT
        AND organization_id = sqlc.arg('organization_id')::UUID
)
-- If either ID does not belong to the organization, the SELECT below returns no
-- rows. sqlc maps that empty RETURNING result to ErrNotFound for the handler.
INSERT INTO
    inventories (
        product_variant_id,
        warehouse_id,
        quantity_on_hand,
        low_stock_threshold,
        is_active
    )
SELECT
    variant.id,
    warehouse.id,
    sqlc.arg('quantity_on_hand')::INT,
    sqlc.narg('low_stock_threshold')::INT,
    sqlc.arg('is_active')::BOOL
FROM
    variant,
    warehouse
ON CONFLICT (product_variant_id, warehouse_id) DO UPDATE
SET
    quantity_on_hand = EXCLUDED.quantity_on_hand,
    low_stock_threshold = EXCLUDED.low_stock_threshold,
    is_active = EXCLUDED.is_active
RETURNING
    product_variant_id,
    warehouse_id,
    quantity_on_hand,
    quantity_reserved,
    quantity_available,
    low_stock_threshold,
    is_active;

-- name: ListInventoryByOrganization :many
SELECT
    i.product_variant_id,
    i.warehouse_id,
    i.quantity_on_hand,
    i.quantity_reserved,
    i.quantity_available,
    i.low_stock_threshold,
    i.is_active,
    pv.product_id,
    pv.sku AS product_variant_sku,
    pv.name AS product_variant_name,
    p.name AS product_name,
    w.name AS warehouse_name
FROM
    inventories i
    JOIN product_variants pv ON pv.id = i.product_variant_id
    JOIN products p ON p.id = pv.product_id
    JOIN warehouses w ON w.id = i.warehouse_id
WHERE
    pv.organization_id = $1
    AND w.organization_id = $1
ORDER BY
    p.name,
    pv.name,
    w.name;

-- name: ListInventoryByProduct :many
SELECT
    i.product_variant_id,
    i.warehouse_id,
    i.quantity_on_hand,
    i.quantity_reserved,
    i.quantity_available,
    i.low_stock_threshold,
    i.is_active,
    pv.product_id,
    pv.sku AS product_variant_sku,
    pv.name AS product_variant_name,
    p.name AS product_name,
    w.name AS warehouse_name
FROM
    inventories i
    JOIN product_variants pv ON pv.id = i.product_variant_id
    JOIN products p ON p.id = pv.product_id
    JOIN warehouses w ON w.id = i.warehouse_id
WHERE
    pv.organization_id = sqlc.arg('organization_id')::UUID
    AND w.organization_id = sqlc.arg('organization_id')::UUID
    AND pv.product_id = sqlc.arg('product_id')::UUID
ORDER BY
    pv.name,
    w.name;

-- name: ListInventoryByVariant :many
SELECT
    i.product_variant_id,
    i.warehouse_id,
    i.quantity_on_hand,
    i.quantity_reserved,
    i.quantity_available,
    i.low_stock_threshold,
    i.is_active,
    pv.product_id,
    pv.sku AS product_variant_sku,
    pv.name AS product_variant_name,
    p.name AS product_name,
    w.name AS warehouse_name
FROM
    inventories i
    JOIN product_variants pv ON pv.id = i.product_variant_id
    JOIN products p ON p.id = pv.product_id
    JOIN warehouses w ON w.id = i.warehouse_id
WHERE
    pv.organization_id = sqlc.arg('organization_id')::UUID
    AND w.organization_id = sqlc.arg('organization_id')::UUID
    AND i.product_variant_id = sqlc.arg('product_variant_id')::UUID
ORDER BY
    w.name;

-- name: GetInventoryByVariantAndWarehouseForOrganization :one
SELECT
    i.product_variant_id,
    i.warehouse_id,
    i.quantity_on_hand,
    i.quantity_reserved,
    i.quantity_available,
    i.low_stock_threshold,
    i.is_active,
    pv.product_id,
    pv.sku AS product_variant_sku,
    pv.name AS product_variant_name,
    p.name AS product_name,
    w.name AS warehouse_name
FROM
    inventories i
    JOIN product_variants pv ON pv.id = i.product_variant_id
    JOIN products p ON p.id = pv.product_id
    JOIN warehouses w ON w.id = i.warehouse_id
WHERE
    pv.organization_id = sqlc.arg('organization_id')::UUID
    AND w.organization_id = sqlc.arg('organization_id')::UUID
    AND i.product_variant_id = sqlc.arg('product_variant_id')::UUID
    AND i.warehouse_id = sqlc.arg('warehouse_id')::BIGINT
LIMIT
    1;
