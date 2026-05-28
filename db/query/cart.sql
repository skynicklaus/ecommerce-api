-- name: CreateCart :one
INSERT INTO
    carts (customer_org_id)
VALUES
    ($1) ON CONFLICT (customer_org_id) DO
UPDATE
SET
    customer_org_id = EXCLUDED.customer_org_id
RETURNING
    *;

-- name: GetCartByCustomerOrgID :one
SELECT
    *
FROM
    carts
WHERE
    customer_org_id = $1;

-- name: GetCartItemDetailsForCustomerOrg :one
SELECT
    i.id AS cart_item_id,
    i.product_variant_id,
    i.quantity,
    i.unit_price,
    i.is_selected AS item_is_selected,
    p.id AS product_id,
    p.name AS product_name,
    p.slug AS product_slug,
    v.name AS variant_name,
    v.sku,
    v.price AS current_price,
    v.is_active AS variant_is_active,
    p.status AS product_status
FROM
    cart_items i
    JOIN cart_shop_groups g ON g.id = i.cart_shop_group_id
    JOIN carts c ON c.id = g.cart_id
    JOIN product_variants v ON v.id = i.product_variant_id
    JOIN products p ON p.id = v.product_id
WHERE
    i.id = sqlc.arg('cart_item_id')
    AND c.customer_org_id = sqlc.arg('customer_org_id');

-- name: GetCartDetails :many
SELECT
    c.id AS cart_id,
    c.customer_org_id,
    g.id AS cart_shop_group_id,
    g.merchant_org_id,
    merchant.name AS merchant_name,
    g.is_selected AS group_is_selected,
    g.subtotal,
    i.id AS cart_item_id,
    i.product_variant_id,
    i.quantity,
    i.unit_price,
    i.is_selected AS item_is_selected,
    p.id AS product_id,
    p.name AS product_name,
    p.slug AS product_slug,
    v.name AS variant_name,
    v.sku,
    v.price AS current_price,
    v.is_active AS variant_is_active,
    p.status AS product_status,
    COALESCE(thumbnail.asset_key, '') AS thumbnail_asset_key,
    COALESCE(thumbnail.source, '') AS thumbnail_source
FROM
    carts c
    JOIN cart_shop_groups g ON g.cart_id = c.id
    JOIN organizations merchant ON merchant.id = g.merchant_org_id
    JOIN cart_items i ON i.cart_shop_group_id = g.id
    JOIN product_variants v ON v.id = i.product_variant_id
    JOIN products p ON p.id = v.product_id
    LEFT JOIN LATERAL (
        SELECT
            pa.asset_key,
            CASE
                WHEN pa.product_variant_id = v.id THEN 'variant'::TEXT
                ELSE 'product'::TEXT
            END AS source
        FROM
            product_assets pa
        WHERE
            pa.product_id = p.id
            AND pa."type" = 'image'
            AND (
                pa.product_variant_id = v.id
                OR pa.product_variant_id IS NULL
            )
        ORDER BY
            CASE
                WHEN pa.product_variant_id = v.id THEN 0
                ELSE 1
            END,
            pa.is_primary DESC,
            pa.sort_order ASC,
            pa.id ASC
        LIMIT
            1
    ) thumbnail ON TRUE
WHERE
    c.customer_org_id = $1
ORDER BY
    merchant.name,
    p.name,
    v.name;
