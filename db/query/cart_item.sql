-- name: GetCartItemForBuyerOrg :one
SELECT
    i.*,
    g.cart_id
FROM
    cart_items i
    JOIN cart_shop_groups g ON g.id = i.cart_shop_group_id
    JOIN carts c ON c.id = g.cart_id
WHERE
    i.id = sqlc.arg('cart_item_id')
    AND c.buyer_org_id = sqlc.arg('buyer_org_id');

-- name: UpsertCartItem :one
INSERT INTO
    cart_items (
        cart_shop_group_id,
        product_variant_id,
        quantity,
        unit_price,
        is_selected
    )
VALUES
    ($1, $2, $3, $4, TRUE) ON CONFLICT (cart_shop_group_id, product_variant_id) DO
UPDATE
SET
    quantity = cart_items.quantity + EXCLUDED.quantity,
    unit_price = EXCLUDED.unit_price,
    is_selected = TRUE
RETURNING
    *;

-- name: UpdateCartItemQuantityForBuyerOrg :one
UPDATE
    cart_items i
SET
    quantity = sqlc.arg('quantity')::SMALLINT
FROM
    cart_shop_groups g,
    carts c
WHERE
    i.id = sqlc.arg('cart_item_id')
    AND i.cart_shop_group_id = g.id
    AND g.cart_id = c.id
    AND c.buyer_org_id = sqlc.arg('buyer_org_id')
RETURNING
    i.*;

-- name: SetCartItemSelectedForBuyerOrg :one
UPDATE
    cart_items i
SET
    is_selected = sqlc.arg('is_selected')::BOOLEAN
FROM
    cart_shop_groups g,
    carts c
WHERE
    i.id = sqlc.arg('cart_item_id')
    AND i.cart_shop_group_id = g.id
    AND g.cart_id = c.id
    AND c.buyer_org_id = sqlc.arg('buyer_org_id')
RETURNING
    i.*;

-- name: DeleteCartItemForBuyerOrg :exec
DELETE FROM
    cart_items i USING cart_shop_groups g,
    carts c
WHERE
    i.id = sqlc.arg('cart_item_id')
    AND i.cart_shop_group_id = g.id
    AND g.cart_id = c.id
    AND c.buyer_org_id = sqlc.arg('buyer_org_id');

-- name: SetCartItemsSelectedByGroupForBuyerOrg :exec
UPDATE
    cart_items i
SET
    is_selected = sqlc.arg('is_selected')::BOOLEAN
FROM
    cart_shop_groups g,
    carts c
WHERE
    i.cart_shop_group_id = g.id
    AND g.id = sqlc.arg('cart_shop_group_id')
    AND g.cart_id = c.id
    AND c.buyer_org_id = sqlc.arg('buyer_org_id');
