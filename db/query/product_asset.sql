-- name: CreateProductAsset :one
INSERT INTO product_assets (
    product_id
    , product_variant_id
    , asset_key
    , type
    , mime_type
    , alt_text
    , sort_order
    , is_primary
    , duration_seconds
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;
