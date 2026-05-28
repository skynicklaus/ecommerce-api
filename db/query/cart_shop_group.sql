-- name: GetOrCreateCartShopGroup :one
INSERT INTO
    cart_shop_groups (cart_id, merchant_org_id)
VALUES
    ($1, $2) ON CONFLICT (cart_id, merchant_org_id) DO
UPDATE
SET
    merchant_org_id = EXCLUDED.merchant_org_id
RETURNING
    *;

-- name: SetCartShopGroupSelectedForCustomerOrg :one
UPDATE
    cart_shop_groups g
SET
    is_selected = sqlc.arg('is_selected')::BOOLEAN
FROM
    carts c
WHERE
    g.id = sqlc.arg('cart_shop_group_id')
    AND g.cart_id = c.id
    AND c.customer_org_id = sqlc.arg('customer_org_id')
RETURNING
    g.*;

-- name: RecalculateCartShopGroupSubtotal :one
UPDATE
    cart_shop_groups g
SET
    subtotal = COALESCE(
        (
            SELECT
                SUM(i.unit_price * i.quantity)
            FROM
                cart_items i
            WHERE
                i.cart_shop_group_id = g.id
        ),
        0
    )
WHERE
    g.id = $1
RETURNING
    *;

-- name: RecalculateCartShopGroupSelection :one
UPDATE
    cart_shop_groups g
SET
    is_selected = EXISTS (
        SELECT
            1
        FROM
            cart_items i
        WHERE
            i.cart_shop_group_id = g.id
    )
    AND NOT EXISTS (
        SELECT
            1
        FROM
            cart_items i
        WHERE
            i.cart_shop_group_id = g.id
            AND i.is_selected = FALSE
    )
WHERE
    g.id = $1
RETURNING
    *;

-- name: DeleteEmptyCartShopGroups :exec
DELETE FROM
    cart_shop_groups g
WHERE
    g.cart_id = $1
    AND NOT EXISTS (
        SELECT
            1
        FROM
            cart_items i
        WHERE
            i.cart_shop_group_id = g.id
    );
