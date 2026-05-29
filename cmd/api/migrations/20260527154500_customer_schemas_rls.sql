-- +goose Up
-- +goose StatementBegin
SET
    lock_timeout = '2s';

ALTER TABLE
    carts ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    cart_shop_groups ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    cart_items ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    checkout_sessions ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    orders ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    order_items ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    inventory_reservations ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    inventory_reservation_items ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    payments ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    payment_transactions ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    order_status_histories ENABLE ROW LEVEL SECURITY;

ALTER TABLE
    carts FORCE ROW LEVEL SECURITY;

ALTER TABLE
    cart_shop_groups FORCE ROW LEVEL SECURITY;

ALTER TABLE
    cart_items FORCE ROW LEVEL SECURITY;

ALTER TABLE
    checkout_sessions FORCE ROW LEVEL SECURITY;

ALTER TABLE
    orders FORCE ROW LEVEL SECURITY;

ALTER TABLE
    order_items FORCE ROW LEVEL SECURITY;

ALTER TABLE
    inventory_reservations FORCE ROW LEVEL SECURITY;

ALTER TABLE
    inventory_reservation_items FORCE ROW LEVEL SECURITY;

ALTER TABLE
    payments FORCE ROW LEVEL SECURITY;

ALTER TABLE
    payment_transactions FORCE ROW LEVEL SECURITY;

ALTER TABLE
    order_status_histories FORCE ROW LEVEL SECURITY;

CREATE OR REPLACE FUNCTION is_current_org_cart(p_cart_id uuid) RETURNS bool AS
$$
SELECT EXISTS (
    SELECT 1
    FROM carts
    WHERE id = p_cart_id
      AND buyer_org_id = current_org_id()
)
$$ LANGUAGE SQL STABLE SECURITY DEFINER
SET search_path = pg_catalog, public;

CREATE OR REPLACE FUNCTION is_current_org_cart_shop_group(p_cart_shop_group_id uuid) RETURNS bool AS
$$
SELECT EXISTS (
    SELECT 1
    FROM cart_shop_groups g
    JOIN carts c ON c.id = g.cart_id
    WHERE g.id = p_cart_shop_group_id
      AND c.buyer_org_id = current_org_id()
)
$$ LANGUAGE SQL STABLE SECURITY DEFINER
SET search_path = pg_catalog, public;

CREATE OR REPLACE FUNCTION is_current_org_order(p_order_id uuid) RETURNS bool AS
$$
SELECT EXISTS (
    SELECT 1
    FROM orders o
    WHERE o.id = p_order_id
      AND (o.buyer_org_id = current_org_id() OR o.merchant_org_id = current_org_id())
)
$$ LANGUAGE SQL STABLE SECURITY DEFINER
SET search_path = pg_catalog, public;

CREATE OR REPLACE FUNCTION is_current_org_inventory_reservation(p_reservation_id uuid) RETURNS bool AS
$$
SELECT EXISTS (
    SELECT 1
    FROM inventory_reservations ir
    WHERE ir.id = p_reservation_id
      AND (ir.buyer_org_id = current_org_id() OR ir.merchant_org_id = current_org_id())
)
$$ LANGUAGE SQL STABLE SECURITY DEFINER
SET search_path = pg_catalog, public;

CREATE OR REPLACE FUNCTION is_current_org_payment(p_payment_id uuid) RETURNS bool AS
$$
SELECT EXISTS (
    SELECT 1
    FROM payments p
    WHERE p.id = p_payment_id
      AND p.buyer_org_id = current_org_id()
)
$$ LANGUAGE SQL STABLE SECURITY DEFINER
SET search_path = pg_catalog, public;

CREATE POLICY org_isolation_read ON carts FOR SELECT USING (
    buyer_org_id = current_org_id()
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON carts FOR INSERT WITH CHECK (
    buyer_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON carts FOR UPDATE USING (
    buyer_org_id = current_org_id()
    OR is_platform_admin()
) WITH CHECK (
    buyer_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON carts FOR DELETE USING (
    buyer_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON cart_shop_groups FOR SELECT USING (
    is_current_org_cart(cart_id)
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON cart_shop_groups FOR INSERT WITH CHECK (
    is_current_org_cart(cart_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON cart_shop_groups FOR UPDATE USING (
    is_current_org_cart(cart_id)
    OR is_platform_admin()
) WITH CHECK (
    is_current_org_cart(cart_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON cart_shop_groups FOR DELETE USING (
    is_current_org_cart(cart_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON cart_items FOR SELECT USING (
    is_current_org_cart_shop_group(cart_shop_group_id)
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON cart_items FOR INSERT WITH CHECK (
    is_current_org_cart_shop_group(cart_shop_group_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON cart_items FOR UPDATE USING (
    is_current_org_cart_shop_group(cart_shop_group_id)
    OR is_platform_admin()
) WITH CHECK (
    is_current_org_cart_shop_group(cart_shop_group_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON cart_items FOR DELETE USING (
    is_current_org_cart_shop_group(cart_shop_group_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON checkout_sessions FOR SELECT USING (
    buyer_org_id = current_org_id()
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON checkout_sessions FOR INSERT WITH CHECK (
    buyer_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON checkout_sessions FOR UPDATE USING (
    buyer_org_id = current_org_id()
    OR is_platform_admin()
) WITH CHECK (
    buyer_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON checkout_sessions FOR DELETE USING (
    buyer_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON orders FOR SELECT USING (
    buyer_org_id = current_org_id()
    OR merchant_org_id = current_org_id()
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON orders FOR INSERT WITH CHECK (
    buyer_org_id = current_org_id()
    OR merchant_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON orders FOR UPDATE USING (
    buyer_org_id = current_org_id()
    OR merchant_org_id = current_org_id()
    OR is_platform_admin()
) WITH CHECK (
    buyer_org_id = current_org_id()
    OR merchant_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON orders FOR DELETE USING (
    buyer_org_id = current_org_id()
    OR merchant_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON order_items FOR SELECT USING (
    is_current_org_order(order_id)
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON order_items FOR INSERT WITH CHECK (
    is_current_org_order(order_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON order_items FOR UPDATE USING (
    is_current_org_order(order_id)
    OR is_platform_admin()
) WITH CHECK (
    is_current_org_order(order_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON order_items FOR DELETE USING (
    is_current_org_order(order_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON inventory_reservations FOR SELECT USING (
    buyer_org_id = current_org_id()
    OR merchant_org_id = current_org_id()
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON inventory_reservations FOR INSERT WITH CHECK (
    buyer_org_id = current_org_id()
    OR merchant_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON inventory_reservations FOR UPDATE USING (
    buyer_org_id = current_org_id()
    OR merchant_org_id = current_org_id()
    OR is_platform_admin()
) WITH CHECK (
    buyer_org_id = current_org_id()
    OR merchant_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON inventory_reservations FOR DELETE USING (
    buyer_org_id = current_org_id()
    OR merchant_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON inventory_reservation_items FOR SELECT USING (
    is_current_org_inventory_reservation(reservation_id)
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON inventory_reservation_items FOR INSERT WITH CHECK (
    is_current_org_inventory_reservation(reservation_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON inventory_reservation_items FOR UPDATE USING (
    is_current_org_inventory_reservation(reservation_id)
    OR is_platform_admin()
) WITH CHECK (
    is_current_org_inventory_reservation(reservation_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON inventory_reservation_items FOR DELETE USING (
    is_current_org_inventory_reservation(reservation_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON payments FOR SELECT USING (
    buyer_org_id = current_org_id()
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON payments FOR INSERT WITH CHECK (
    buyer_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON payments FOR UPDATE USING (
    buyer_org_id = current_org_id()
    OR is_platform_admin()
) WITH CHECK (
    buyer_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON payments FOR DELETE USING (
    buyer_org_id = current_org_id()
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON payment_transactions FOR SELECT USING (
    is_current_org_payment(payment_id)
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON payment_transactions FOR INSERT WITH CHECK (
    is_current_org_payment(payment_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON payment_transactions FOR UPDATE USING (
    is_current_org_payment(payment_id)
    OR is_platform_admin()
) WITH CHECK (
    is_current_org_payment(payment_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON payment_transactions FOR DELETE USING (
    is_current_org_payment(payment_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_read ON order_status_histories FOR SELECT USING (
    is_current_org_order(order_id)
    OR is_platform_user()
);

CREATE POLICY org_isolation_write ON order_status_histories FOR INSERT WITH CHECK (
    is_current_org_order(order_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_update ON order_status_histories FOR UPDATE USING (
    is_current_org_order(order_id)
    OR is_platform_admin()
) WITH CHECK (
    is_current_org_order(order_id)
    OR is_platform_admin()
);

CREATE POLICY org_isolation_delete ON order_status_histories FOR DELETE USING (
    is_current_org_order(order_id)
    OR is_platform_admin()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP POLICY IF EXISTS org_isolation_delete ON order_status_histories;
DROP POLICY IF EXISTS org_isolation_update ON order_status_histories;
DROP POLICY IF EXISTS org_isolation_write ON order_status_histories;
DROP POLICY IF EXISTS org_isolation_read ON order_status_histories;
DROP POLICY IF EXISTS org_isolation_delete ON payment_transactions;
DROP POLICY IF EXISTS org_isolation_update ON payment_transactions;
DROP POLICY IF EXISTS org_isolation_write ON payment_transactions;
DROP POLICY IF EXISTS org_isolation_read ON payment_transactions;
DROP POLICY IF EXISTS org_isolation_delete ON payments;
DROP POLICY IF EXISTS org_isolation_update ON payments;
DROP POLICY IF EXISTS org_isolation_write ON payments;
DROP POLICY IF EXISTS org_isolation_read ON payments;
DROP POLICY IF EXISTS org_isolation_delete ON inventory_reservation_items;
DROP POLICY IF EXISTS org_isolation_update ON inventory_reservation_items;
DROP POLICY IF EXISTS org_isolation_write ON inventory_reservation_items;
DROP POLICY IF EXISTS org_isolation_read ON inventory_reservation_items;
DROP POLICY IF EXISTS org_isolation_delete ON inventory_reservations;
DROP POLICY IF EXISTS org_isolation_update ON inventory_reservations;
DROP POLICY IF EXISTS org_isolation_write ON inventory_reservations;
DROP POLICY IF EXISTS org_isolation_read ON inventory_reservations;
DROP POLICY IF EXISTS org_isolation_delete ON order_items;
DROP POLICY IF EXISTS org_isolation_update ON order_items;
DROP POLICY IF EXISTS org_isolation_write ON order_items;
DROP POLICY IF EXISTS org_isolation_read ON order_items;
DROP POLICY IF EXISTS org_isolation_delete ON orders;
DROP POLICY IF EXISTS org_isolation_update ON orders;
DROP POLICY IF EXISTS org_isolation_write ON orders;
DROP POLICY IF EXISTS org_isolation_read ON orders;
DROP POLICY IF EXISTS org_isolation_delete ON checkout_sessions;
DROP POLICY IF EXISTS org_isolation_update ON checkout_sessions;
DROP POLICY IF EXISTS org_isolation_write ON checkout_sessions;
DROP POLICY IF EXISTS org_isolation_read ON checkout_sessions;
DROP POLICY IF EXISTS org_isolation_delete ON cart_items;
DROP POLICY IF EXISTS org_isolation_update ON cart_items;
DROP POLICY IF EXISTS org_isolation_write ON cart_items;
DROP POLICY IF EXISTS org_isolation_read ON cart_items;
DROP POLICY IF EXISTS org_isolation_delete ON cart_shop_groups;
DROP POLICY IF EXISTS org_isolation_update ON cart_shop_groups;
DROP POLICY IF EXISTS org_isolation_write ON cart_shop_groups;
DROP POLICY IF EXISTS org_isolation_read ON cart_shop_groups;
DROP POLICY IF EXISTS org_isolation_delete ON carts;
DROP POLICY IF EXISTS org_isolation_update ON carts;
DROP POLICY IF EXISTS org_isolation_write ON carts;
DROP POLICY IF EXISTS org_isolation_read ON carts;

DROP FUNCTION IF EXISTS is_current_org_payment(uuid);
DROP FUNCTION IF EXISTS is_current_org_inventory_reservation(uuid);
DROP FUNCTION IF EXISTS is_current_org_order(uuid);
DROP FUNCTION IF EXISTS is_current_org_cart_shop_group(uuid);
DROP FUNCTION IF EXISTS is_current_org_cart(uuid);

ALTER TABLE order_status_histories NO FORCE ROW LEVEL SECURITY;
ALTER TABLE payment_transactions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE payments NO FORCE ROW LEVEL SECURITY;
ALTER TABLE inventory_reservation_items NO FORCE ROW LEVEL SECURITY;
ALTER TABLE inventory_reservations NO FORCE ROW LEVEL SECURITY;
ALTER TABLE order_items NO FORCE ROW LEVEL SECURITY;
ALTER TABLE orders NO FORCE ROW LEVEL SECURITY;
ALTER TABLE checkout_sessions NO FORCE ROW LEVEL SECURITY;
ALTER TABLE cart_items NO FORCE ROW LEVEL SECURITY;
ALTER TABLE cart_shop_groups NO FORCE ROW LEVEL SECURITY;
ALTER TABLE carts NO FORCE ROW LEVEL SECURITY;

ALTER TABLE order_status_histories DISABLE ROW LEVEL SECURITY;
ALTER TABLE payment_transactions DISABLE ROW LEVEL SECURITY;
ALTER TABLE payments DISABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_reservation_items DISABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_reservations DISABLE ROW LEVEL SECURITY;
ALTER TABLE order_items DISABLE ROW LEVEL SECURITY;
ALTER TABLE orders DISABLE ROW LEVEL SECURITY;
ALTER TABLE checkout_sessions DISABLE ROW LEVEL SECURITY;
ALTER TABLE cart_items DISABLE ROW LEVEL SECURITY;
ALTER TABLE cart_shop_groups DISABLE ROW LEVEL SECURITY;
ALTER TABLE carts DISABLE ROW LEVEL SECURITY;
-- +goose StatementEnd
