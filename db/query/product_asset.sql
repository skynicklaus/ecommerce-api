-- name: CreateProductAsset :one
INSERT INTO
    product_assets (
        product_id,
        product_variant_id,
        asset_key,
        "type",
        mime_type,
        alt_text,
        sort_order,
        is_primary,
        duration_seconds
    )
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING
    *;

-- name: ListProductAssetsByProductID :many
SELECT
    id,
    product_id,
    product_variant_id,
    asset_key,
    "type",
    mime_type,
    alt_text,
    sort_order,
    is_primary,
    duration_seconds
FROM
    product_assets
WHERE
    product_id = $1
ORDER BY
    sort_order;

-- name: ListProductAssetsByProductIDs :many
SELECT
    id,
    product_id,
    product_variant_id,
    asset_key,
    "type",
    mime_type,
    alt_text,
    sort_order,
    is_primary,
    duration_seconds
FROM
    product_assets
WHERE
    product_id = ANY (sqlc.arg ('product_ids')::UUID [])
ORDER BY
    product_id,
    sort_order;
