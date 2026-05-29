-- name: ListSelectedCartItemsForCheckout :many
SELECT
    c.id AS cart_id,
    g.id AS cart_shop_group_id,
    i.id AS cart_item_id,
    g.merchant_org_id,
    i.product_variant_id,
    i.quantity,
    i.unit_price AS cart_unit_price,
    pv.price AS current_unit_price,
    pv.track_inventory,
    pv.is_active AS variant_is_active,
    pv.sku,
    pv.name AS variant_name,
    p.id AS product_id,
    p.name AS product_name,
    p.slug AS product_slug,
    p.status AS product_status,
    COALESCE(
        (
        SELECT
            pa.asset_key
        FROM
            product_assets pa
        WHERE
            pa.product_id = p.id
            AND pa.type = 'image'
            AND (
                pa.product_variant_id = pv.id
                OR pa.product_variant_id IS NULL
            )
        ORDER BY
            (pa.product_variant_id = pv.id) DESC,
            pa.is_primary DESC,
            pa.sort_order,
            pa.id
        LIMIT
            1
        ),
        ''
    )::TEXT AS thumbnail_asset_key
FROM
    carts c
    JOIN cart_shop_groups g ON g.cart_id = c.id
    JOIN cart_items i ON i.cart_shop_group_id = g.id
    JOIN product_variants pv ON pv.id = i.product_variant_id
    JOIN products p ON p.id = pv.product_id
WHERE
    c.buyer_org_id = sqlc.arg('buyer_org_id')::UUID
    AND i.is_selected = TRUE
ORDER BY
    g.merchant_org_id,
    i.created_at,
    i.id
FOR UPDATE OF
    i;

-- name: CountSelectedCartItemsForCheckout :one
SELECT
    COUNT(*)
FROM
    carts c
    JOIN cart_shop_groups g ON g.cart_id = c.id
    JOIN cart_items i ON i.cart_shop_group_id = g.id
WHERE
    c.buyer_org_id = sqlc.arg('buyer_org_id')::UUID
    AND i.is_selected = TRUE;

-- name: DeleteSelectedCartItemsForCheckout :many
DELETE FROM
    cart_items i USING cart_shop_groups g,
    carts c
WHERE
    i.cart_shop_group_id = g.id
    AND g.cart_id = c.id
    AND c.buyer_org_id = sqlc.arg('buyer_org_id')::UUID
    AND i.is_selected = TRUE
    AND i.id = ANY(sqlc.arg('cart_item_ids')::UUID [])
RETURNING
    i.id,
    i.cart_shop_group_id,
    i.product_variant_id;
