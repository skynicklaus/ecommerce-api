-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS carts (
    id UUID NOT NULL PRIMARY KEY DEFAULT UUIDV7(),
    buyer_org_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_carts_buyer_org_id FOREIGN KEY (buyer_org_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS cart_shop_groups (
    id UUID NOT NULL PRIMARY KEY DEFAULT UUIDV7(),
    cart_id UUID NOT NULL,
    merchant_org_id UUID NOT NULL,
    is_selected BOOLEAN NOT NULL DEFAULT false,
    subtotal NUMERIC(19, 4) NOT NULL DEFAULT 0.00,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_cart_shop_groups_cart_id FOREIGN KEY (cart_id) REFERENCES carts(id) ON DELETE CASCADE,
    CONSTRAINT fk_cart_shop_groups_merchant_org_id FOREIGN KEY (merchant_org_id) REFERENCES organizations(id) ON DELETE CASCADE,
    CONSTRAINT check_cart_shop_groups_subtotal_non_negative CHECK (subtotal >= 0)
);

CREATE TABLE IF NOT EXISTS cart_items (
    id UUID NOT NULL PRIMARY KEY DEFAULT UUIDV7(),
    cart_shop_group_id UUID NOT NULL,
    product_variant_id UUID NOT NULL,
    quantity SMALLINT NOT NULL,
    unit_price NUMERIC(19, 4) NOT NULL,
    is_selected BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_cart_items_cart_shop_group_id FOREIGN KEY (cart_shop_group_id) REFERENCES cart_shop_groups(id) ON DELETE CASCADE,
    CONSTRAINT fk_cart_items_product_variant_id FOREIGN KEY (product_variant_id) REFERENCES product_variants(id),
    CONSTRAINT check_cart_items_quantity_positive CHECK (quantity > 0),
    CONSTRAINT check_cart_items_unit_price_non_negative CHECK (unit_price >= 0)
);

CREATE TABLE IF NOT EXISTS checkout_sessions (
    id UUID NOT NULL PRIMARY KEY DEFAULT UUIDV7(),
    buyer_customer_id UUID NOT NULL,
    buyer_org_id UUID NOT NULL,
    buyer_member_id UUID NOT NULL,
    "status" TEXT NOT NULL DEFAULT 'pending',
    idempotency_key TEXT,
    checkout_fingerprint TEXT NOT NULL,
    subtotal NUMERIC(19, 4) NOT NULL DEFAULT 0,
    tax_total NUMERIC(19, 4) NOT NULL DEFAULT 0,
    shipping_total NUMERIC(19, 4) NOT NULL DEFAULT 0,
    discount_total NUMERIC(19, 4) NOT NULL DEFAULT 0,
    grand_total NUMERIC(19, 4) NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'MYR',
    expires_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_checkout_sessions_buyer_customer_id FOREIGN KEY (buyer_customer_id) REFERENCES customers(id) ON DELETE RESTRICT,
    CONSTRAINT fk_checkout_sessions_buyer_org_id FOREIGN KEY (buyer_org_id) REFERENCES organizations(id) ON DELETE RESTRICT,
    CONSTRAINT fk_checkout_sessions_buyer_member_id FOREIGN KEY (buyer_member_id) REFERENCES members(id) ON DELETE RESTRICT,
    CONSTRAINT check_checkout_sessions_status CHECK (
        "status" IN (
            'pending',
            'reserved',
            'payment_pending',
            'completed',
            'cancelled',
            'expired',
            'failed'
        )
    ),
    CONSTRAINT check_checkout_sessions_money_non_negative CHECK (
        subtotal >= 0
        AND tax_total >= 0
        AND shipping_total >= 0
        AND discount_total >= 0
        AND grand_total >= 0
    ),
    CONSTRAINT check_checkout_sessions_currency CHECK (LENGTH(currency) = 3),
    CONSTRAINT check_checkout_sessions_terminal_timestamps CHECK (
        (
            "status" <> 'completed'
            OR completed_at IS NOT NULL
        )
        AND (
            "status" <> 'cancelled'
            OR cancelled_at IS NOT NULL
        )
    )
);

CREATE TABLE IF NOT EXISTS orders (
    id UUID NOT NULL PRIMARY KEY DEFAULT UUIDV7(),
    checkout_session_id UUID NOT NULL,
    merchant_org_id UUID NOT NULL,
    buyer_customer_id UUID NOT NULL,
    buyer_org_id UUID NOT NULL,
    buyer_member_id UUID NOT NULL,
    order_number TEXT NOT NULL,
    "status" TEXT NOT NULL DEFAULT 'pending_payment',
    payment_status TEXT NOT NULL DEFAULT 'unpaid',
    fulfillment_status TEXT NOT NULL DEFAULT 'unfulfilled',
    customer_email TEXT NOT NULL,
    customer_name TEXT NOT NULL,
    billing_address_snapshot JSONB,
    shipping_address_snapshot JSONB NOT NULL,
    subtotal NUMERIC(19, 4) NOT NULL DEFAULT 0,
    tax_total NUMERIC(19, 4) NOT NULL DEFAULT 0,
    shipping_total NUMERIC(19, 4) NOT NULL DEFAULT 0,
    shipping_discount NUMERIC(19, 4) NOT NULL DEFAULT 0,
    coupon_discount NUMERIC(19, 4) NOT NULL DEFAULT 0,
    grand_total NUMERIC(19, 4) NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'MYR',
    placed_at TIMESTAMPTZ,
    paid_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_orders_checkout_session_id FOREIGN KEY (checkout_session_id) REFERENCES checkout_sessions(id) ON DELETE RESTRICT,
    CONSTRAINT fk_orders_merchant_org_id FOREIGN KEY (merchant_org_id) REFERENCES organizations(id) ON DELETE RESTRICT,
    CONSTRAINT fk_orders_buyer_customer_id FOREIGN KEY (buyer_customer_id) REFERENCES customers(id) ON DELETE RESTRICT,
    CONSTRAINT fk_orders_buyer_org_id FOREIGN KEY (buyer_org_id) REFERENCES organizations(id) ON DELETE RESTRICT,
    CONSTRAINT fk_orders_buyer_member_id FOREIGN KEY (buyer_member_id) REFERENCES members(id) ON DELETE RESTRICT,
    CONSTRAINT check_orders_status CHECK (
        "status" IN (
            'pending_payment',
            'placed',
            'processing',
            'cancelled',
            'expired',
            'completed'
        )
    ),
    CONSTRAINT check_orders_payment_status CHECK (
        payment_status IN (
            'unpaid',
            'authorized',
            'paid',
            'failed',
            'partially_refunded',
            'refunded'
        )
    ),
    CONSTRAINT check_orders_fulfillment_status CHECK (
        fulfillment_status IN (
            'unfulfilled',
            'processing',
            'shipped',
            'delivered',
            'returned'
        )
    ),
    CONSTRAINT check_orders_money_non_negative CHECK (
        subtotal >= 0
        AND tax_total >= 0
        AND shipping_total >= 0
        AND shipping_discount >= 0
        AND coupon_discount >= 0
        AND grand_total >= 0
    ),
    CONSTRAINT check_orders_currency CHECK (LENGTH(currency) = 3)
);

CREATE TABLE IF NOT EXISTS order_items (
    id UUID NOT NULL PRIMARY KEY DEFAULT UUIDV7(),
    order_id UUID NOT NULL,
    product_id UUID,
    product_variant_id UUID,
    warehouse_id BIGINT,
    product_name TEXT NOT NULL,
    product_slug TEXT,
    variant_name TEXT NOT NULL,
    sku TEXT NOT NULL,
    variant_attributes JSONB,
    thumbnail_asset_key TEXT,
    quantity INTEGER NOT NULL,
    unit_price NUMERIC(19, 4) NOT NULL,
    subtotal NUMERIC(19, 4) NOT NULL,
    discount_total NUMERIC(19, 4) NOT NULL DEFAULT 0,
    tax_total NUMERIC(19, 4) NOT NULL DEFAULT 0,
    total NUMERIC(19, 4) NOT NULL,
    currency TEXT NOT NULL DEFAULT 'MYR',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_order_items_order_id FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE,
    CONSTRAINT fk_order_items_product_id FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE
    SET
        NULL,
        CONSTRAINT fk_order_items_product_variant_id FOREIGN KEY (product_variant_id) REFERENCES product_variants(id) ON DELETE
    SET
        NULL,
        CONSTRAINT fk_order_items_warehouse_id FOREIGN KEY (warehouse_id) REFERENCES warehouses(id) ON DELETE
    SET
        NULL,
        CONSTRAINT check_order_items_quantity_positive CHECK (quantity > 0),
        CONSTRAINT check_order_items_money_non_negative CHECK (
            unit_price >= 0
            AND subtotal >= 0
            AND discount_total >= 0
            AND tax_total >= 0
            AND total >= 0
        ),
        CONSTRAINT check_order_items_currency CHECK (LENGTH(currency) = 3)
);

CREATE TABLE IF NOT EXISTS inventory_reservations (
    id UUID NOT NULL PRIMARY KEY DEFAULT UUIDV7(),
    checkout_session_id UUID NOT NULL,
    order_id UUID NOT NULL,
    buyer_org_id UUID NOT NULL,
    merchant_org_id UUID NOT NULL,
    "status" TEXT NOT NULL DEFAULT 'active',
    expires_at TIMESTAMPTZ NOT NULL,
    confirmed_at TIMESTAMPTZ,
    released_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_inventory_reservations_checkout_session_id FOREIGN KEY (checkout_session_id) REFERENCES checkout_sessions(id) ON DELETE RESTRICT,
    CONSTRAINT fk_inventory_reservations_order_id FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE RESTRICT,
    CONSTRAINT fk_inventory_reservations_buyer_org_id FOREIGN KEY (buyer_org_id) REFERENCES organizations(id) ON DELETE RESTRICT,
    CONSTRAINT fk_inventory_reservations_merchant_org_id FOREIGN KEY (merchant_org_id) REFERENCES organizations(id) ON DELETE RESTRICT,
    CONSTRAINT check_inventory_reservations_status CHECK (
        "status" IN (
            'active',
            'confirmed',
            'released',
            'expired',
            'cancelled'
        )
    )
);

CREATE TABLE IF NOT EXISTS inventory_reservation_items (
    id UUID NOT NULL PRIMARY KEY DEFAULT UUIDV7(),
    reservation_id UUID NOT NULL,
    order_item_id UUID NOT NULL,
    product_variant_id UUID NOT NULL,
    warehouse_id BIGINT NOT NULL,
    quantity INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_inventory_reservation_items_reservation_id FOREIGN KEY (reservation_id) REFERENCES inventory_reservations(id) ON DELETE CASCADE,
    CONSTRAINT fk_inventory_reservation_items_order_item_id FOREIGN KEY (order_item_id) REFERENCES order_items(id) ON DELETE RESTRICT,
    CONSTRAINT fk_inventory_reservation_items_product_variant_id FOREIGN KEY (product_variant_id) REFERENCES product_variants(id) ON DELETE RESTRICT,
    CONSTRAINT fk_inventory_reservation_items_warehouse_id FOREIGN KEY (warehouse_id) REFERENCES warehouses(id) ON DELETE RESTRICT,
    CONSTRAINT check_inventory_reservation_items_quantity_positive CHECK (quantity > 0)
);

CREATE TABLE IF NOT EXISTS order_status_histories (
    id UUID NOT NULL PRIMARY KEY DEFAULT UUIDV7(),
    order_id UUID NOT NULL,
    from_status TEXT,
    to_status TEXT NOT NULL,
    note TEXT,
    actor_type TEXT NOT NULL DEFAULT 'system',
    changed_by_member_id UUID,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_order_status_histories_order_id FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE,
    CONSTRAINT fk_order_status_histories_changed_by_member_id FOREIGN KEY (changed_by_member_id) REFERENCES members(id) ON DELETE
    SET
        NULL,
        CONSTRAINT check_order_status_histories_actor_type CHECK (
            actor_type IN (
                'customer',
                'merchant_member',
                'platform_member',
                'system',
                'payment_provider'
            )
        )
);

CREATE TABLE IF NOT EXISTS payments (
    id UUID NOT NULL PRIMARY KEY DEFAULT UUIDV7(),
    checkout_session_id UUID NOT NULL,
    buyer_org_id UUID NOT NULL,
    provider TEXT NOT NULL,
    provider_payment_id TEXT,
    "status" TEXT NOT NULL DEFAULT 'pending',
    amount NUMERIC(19, 4) NOT NULL,
    currency TEXT NOT NULL DEFAULT 'MYR',
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_payments_checkout_session_id FOREIGN KEY (checkout_session_id) REFERENCES checkout_sessions(id) ON DELETE RESTRICT,
    CONSTRAINT fk_payments_buyer_org_id FOREIGN KEY (buyer_org_id) REFERENCES organizations(id) ON DELETE RESTRICT,
    CONSTRAINT check_payments_status CHECK (
        "status" IN (
            'pending',
            'requires_action',
            'authorized',
            'succeeded',
            'failed',
            'cancelled',
            'partially_refunded',
            'refunded'
        )
    ),
    CONSTRAINT check_payments_amount_non_negative CHECK (amount >= 0),
    CONSTRAINT check_payments_currency CHECK (LENGTH(currency) = 3)
);

CREATE TABLE IF NOT EXISTS payment_transactions (
    id UUID NOT NULL PRIMARY KEY DEFAULT UUIDV7(),
    payment_id UUID NOT NULL,
    "type" TEXT NOT NULL,
    "status" TEXT NOT NULL,
    provider TEXT NOT NULL,
    provider_ref TEXT,
    amount NUMERIC(19, 4) NOT NULL,
    currency TEXT NOT NULL DEFAULT 'MYR',
    failure_code TEXT,
    failure_message TEXT,
    metadata JSONB,
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_payment_transactions_payment_id FOREIGN KEY (payment_id) REFERENCES payments(id) ON DELETE CASCADE,
    CONSTRAINT check_payment_transactions_type CHECK (
        "type" IN (
            'authorize',
            'capture',
            'sale',
            'refund',
            'void'
        )
    ),
    CONSTRAINT check_payment_transactions_status CHECK (
        "status" IN (
            'pending',
            'succeeded',
            'failed',
            'cancelled'
        )
    ),
    CONSTRAINT check_payment_transactions_amount_non_negative CHECK (amount >= 0),
    CONSTRAINT check_payment_transactions_currency CHECK (LENGTH(currency) = 3)
);

CREATE
OR REPLACE FUNCTION VALIDATE_CART_BUYER_ORG() RETURNS TRIGGER AS
$$
BEGIN
IF NOT EXISTS (
    SELECT
        1
    FROM
        organizations
    WHERE
        id = NEW.buyer_org_id
        AND capability = 'buyer'
) THEN RAISE EXCEPTION 'cart buyer_org_id must reference a buyer-capable organization';

END IF;

RETURN NEW;

END;

$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION VALIDATE_CART_SHOP_GROUP_MERCHANT_ORG() RETURNS TRIGGER AS
$$
BEGIN
IF NOT EXISTS (
    SELECT
        1
    FROM
        organizations
    WHERE
        id = NEW.merchant_org_id
        AND capability = 'seller'
) THEN RAISE EXCEPTION 'cart merchant_org_id must reference a seller-capable organization';

END IF;

RETURN NEW;

END;

$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION VALIDATE_BUYER_CONTEXT(
    p_buyer_customer_id UUID,
    p_buyer_org_id UUID,
    p_buyer_member_id UUID
) RETURNS VOID AS
$$
BEGIN
IF NOT EXISTS (
    SELECT
        1
    FROM
        members m
        JOIN customers c ON c.identity_id = m.identity_id
        JOIN organizations o ON o.id = m.organization_id
    WHERE
        c.id = p_buyer_customer_id
        AND m.id = p_buyer_member_id
        AND m.organization_id = p_buyer_org_id
        AND o.capability = 'buyer'
) THEN RAISE EXCEPTION 'buyer context must reference a buyer-capable org and matching customer/member identity';

END IF;

END;

$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION VALIDATE_CHECKOUT_SESSION_BUYER_CONTEXT() RETURNS TRIGGER AS
$$
BEGIN
PERFORM VALIDATE_BUYER_CONTEXT(
    NEW.buyer_customer_id,
    NEW.buyer_org_id,
    NEW.buyer_member_id
);

RETURN NEW;

END;

$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION VALIDATE_ORDER_ORGS() RETURNS TRIGGER AS
$$
BEGIN
PERFORM VALIDATE_BUYER_CONTEXT(
    NEW.buyer_customer_id,
    NEW.buyer_org_id,
    NEW.buyer_member_id
);

IF NOT EXISTS (
    SELECT
        1
    FROM
        organizations
    WHERE
        id = NEW.merchant_org_id
        AND capability = 'seller'
) THEN RAISE EXCEPTION 'order merchant_org_id must reference a seller-capable organization';

END IF;

IF NOT EXISTS (
    SELECT
        1
    FROM
        checkout_sessions cs
    WHERE
        cs.id = NEW.checkout_session_id
        AND cs.buyer_customer_id = NEW.buyer_customer_id
        AND cs.buyer_org_id = NEW.buyer_org_id
        AND cs.buyer_member_id = NEW.buyer_member_id
) THEN RAISE EXCEPTION 'order checkout_session_id must belong to the same buyer context';

END IF;

RETURN NEW;

END;

$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION VALIDATE_INVENTORY_RESERVATION_ORGS() RETURNS TRIGGER AS
$$
BEGIN
IF NOT EXISTS (
    SELECT
        1
    FROM
        organizations
    WHERE
        id = NEW.buyer_org_id
        AND capability = 'buyer'
) THEN RAISE EXCEPTION 'inventory reservation buyer_org_id must reference a buyer-capable organization';

END IF;

IF NOT EXISTS (
    SELECT
        1
    FROM
        organizations
    WHERE
        id = NEW.merchant_org_id
        AND capability = 'seller'
) THEN RAISE EXCEPTION 'inventory reservation merchant_org_id must reference a seller-capable organization';

END IF;

IF NOT EXISTS (
    SELECT
        1
    FROM
        orders o
    WHERE
        o.id = NEW.order_id
        AND o.checkout_session_id = NEW.checkout_session_id
        AND o.buyer_org_id = NEW.buyer_org_id
        AND o.merchant_org_id = NEW.merchant_org_id
) THEN RAISE EXCEPTION 'inventory reservation order_id must belong to the same checkout/buyer/merchant';

END IF;

RETURN NEW;

END;

$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION VALIDATE_INVENTORY_RESERVATION_ITEM() RETURNS TRIGGER AS
$$
DECLARE
order_item_quantity INTEGER;

reserved_item_quantity INTEGER;

BEGIN
SELECT
    oi.quantity INTO order_item_quantity
FROM
    inventory_reservations ir
    JOIN order_items oi ON oi.id = NEW.order_item_id
    JOIN orders o ON o.id = oi.order_id
WHERE
    ir.id = NEW.reservation_id
    AND o.id = ir.order_id
    AND oi.product_variant_id = NEW.product_variant_id
    AND (
        oi.warehouse_id IS NULL
        OR oi.warehouse_id = NEW.warehouse_id
    ) FOR
UPDATE
    OF oi;

IF order_item_quantity IS NULL THEN RAISE EXCEPTION 'inventory reservation item must match its reservation order item and inventory bucket';

END IF;

SELECT
    COALESCE(SUM(iri.quantity), 0) INTO reserved_item_quantity
FROM
    inventory_reservation_items iri
WHERE
    iri.reservation_id = NEW.reservation_id
    AND iri.order_item_id = NEW.order_item_id
    AND iri.id <> NEW.id;

IF reserved_item_quantity + NEW.quantity > order_item_quantity THEN RAISE EXCEPTION 'inventory reservation item quantity cannot exceed order item quantity';

END IF;

RETURN NEW;

END;

$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION VALIDATE_PAYMENT_BUYER_ORG() RETURNS TRIGGER AS
$$
BEGIN
IF NOT EXISTS (
    SELECT
        1
    FROM
        checkout_sessions cs
    WHERE
        cs.id = NEW.checkout_session_id
        AND cs.buyer_org_id = NEW.buyer_org_id
) THEN RAISE EXCEPTION 'payment checkout_session_id must belong to buyer_org_id';

END IF;

RETURN NEW;

END;

$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION VALIDATE_ORDER_ITEM_MERCHANT_REFS() RETURNS TRIGGER AS
$$
DECLARE
order_merchant_org_id UUID;

BEGIN
SELECT
    merchant_org_id INTO order_merchant_org_id
FROM
    orders
WHERE
    id = NEW.order_id;

IF order_merchant_org_id IS NULL THEN RAISE EXCEPTION 'order item order_id must reference an existing order';

END IF;

IF NEW.product_id IS NOT NULL
AND NOT EXISTS (
    SELECT
        1
    FROM
        products p
    WHERE
        p.id = NEW.product_id
        AND p.organization_id = order_merchant_org_id
) THEN RAISE EXCEPTION 'order item product_id must belong to the order merchant';

END IF;

IF NEW.product_variant_id IS NOT NULL
AND NOT EXISTS (
    SELECT
        1
    FROM
        product_variants pv
    WHERE
        pv.id = NEW.product_variant_id
        AND pv.organization_id = order_merchant_org_id
        AND (
            NEW.product_id IS NULL
            OR pv.product_id = NEW.product_id
        )
) THEN RAISE EXCEPTION 'order item product_variant_id must belong to the order merchant and product';

END IF;

IF NEW.warehouse_id IS NOT NULL
AND NOT EXISTS (
    SELECT
        1
    FROM
        warehouses w
    WHERE
        w.id = NEW.warehouse_id
        AND w.organization_id = order_merchant_org_id
) THEN RAISE EXCEPTION 'order item warehouse_id must belong to the order merchant';

END IF;

RETURN NEW;

END;

$$
LANGUAGE plpgsql;

CREATE TRIGGER trg_carts_validate_buyer_org BEFORE
INSERT
    OR
UPDATE
    OF buyer_org_id ON carts FOR EACH ROW EXECUTE FUNCTION VALIDATE_CART_BUYER_ORG();

CREATE TRIGGER trg_cart_shop_groups_validate_merchant_org BEFORE
INSERT
    OR
UPDATE
    OF merchant_org_id ON cart_shop_groups FOR EACH ROW EXECUTE FUNCTION VALIDATE_CART_SHOP_GROUP_MERCHANT_ORG();

CREATE TRIGGER trg_carts_updated_at BEFORE
UPDATE
    ON carts FOR EACH ROW EXECUTE FUNCTION SET_UPDATED_AT();

CREATE TRIGGER trg_cart_shop_groups_updated_at BEFORE
UPDATE
    ON cart_shop_groups FOR EACH ROW EXECUTE FUNCTION SET_UPDATED_AT();

CREATE TRIGGER trg_cart_items_updated_at BEFORE
UPDATE
    ON cart_items FOR EACH ROW EXECUTE FUNCTION SET_UPDATED_AT();

CREATE TRIGGER trg_checkout_sessions_validate_buyer_context BEFORE
INSERT
    OR
UPDATE
    OF buyer_customer_id,
    buyer_org_id,
    buyer_member_id ON checkout_sessions FOR EACH ROW EXECUTE FUNCTION VALIDATE_CHECKOUT_SESSION_BUYER_CONTEXT();

CREATE TRIGGER trg_orders_validate_orgs BEFORE
INSERT
    OR
UPDATE
    OF checkout_session_id,
    merchant_org_id,
    buyer_customer_id,
    buyer_org_id,
    buyer_member_id ON orders FOR EACH ROW EXECUTE FUNCTION VALIDATE_ORDER_ORGS();

CREATE TRIGGER trg_inventory_reservations_validate_orgs BEFORE
INSERT
    OR
UPDATE
    OF checkout_session_id,
    order_id,
    buyer_org_id,
    merchant_org_id ON inventory_reservations FOR EACH ROW EXECUTE FUNCTION VALIDATE_INVENTORY_RESERVATION_ORGS();

CREATE TRIGGER trg_inventory_reservation_items_validate BEFORE
INSERT
    OR
UPDATE
    OF reservation_id,
    order_item_id,
    product_variant_id,
    warehouse_id,
    quantity ON inventory_reservation_items FOR EACH ROW EXECUTE FUNCTION VALIDATE_INVENTORY_RESERVATION_ITEM();

CREATE TRIGGER trg_payments_validate_buyer_org BEFORE
INSERT
    OR
UPDATE
    OF checkout_session_id,
    buyer_org_id ON payments FOR EACH ROW EXECUTE FUNCTION VALIDATE_PAYMENT_BUYER_ORG();

CREATE TRIGGER trg_checkout_sessions_updated_at BEFORE
UPDATE
    ON checkout_sessions FOR EACH ROW EXECUTE FUNCTION SET_UPDATED_AT();

CREATE TRIGGER trg_orders_updated_at BEFORE
UPDATE
    ON orders FOR EACH ROW EXECUTE FUNCTION SET_UPDATED_AT();

CREATE TRIGGER trg_payments_updated_at BEFORE
UPDATE
    ON payments FOR EACH ROW EXECUTE FUNCTION SET_UPDATED_AT();

CREATE TRIGGER trg_order_items_validate_merchant_refs BEFORE
INSERT
    OR
UPDATE
    OF order_id,
    product_id,
    product_variant_id,
    warehouse_id ON order_items FOR EACH ROW EXECUTE FUNCTION VALIDATE_ORDER_ITEM_MERCHANT_REFS();

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_order_items_validate_merchant_refs ON order_items;

DROP TRIGGER IF EXISTS trg_payments_updated_at ON payments;

DROP TRIGGER IF EXISTS trg_orders_updated_at ON orders;

DROP TRIGGER IF EXISTS trg_checkout_sessions_updated_at ON checkout_sessions;

DROP TRIGGER IF EXISTS trg_payments_validate_buyer_org ON payments;

DROP TRIGGER IF EXISTS trg_inventory_reservation_items_validate ON inventory_reservation_items;

DROP TRIGGER IF EXISTS trg_inventory_reservations_validate_orgs ON inventory_reservations;

DROP TRIGGER IF EXISTS trg_orders_validate_orgs ON orders;

DROP TRIGGER IF EXISTS trg_checkout_sessions_validate_buyer_context ON checkout_sessions;

DROP TRIGGER IF EXISTS trg_cart_items_updated_at ON cart_items;

DROP TRIGGER IF EXISTS trg_cart_shop_groups_updated_at ON cart_shop_groups;

DROP TRIGGER IF EXISTS trg_carts_updated_at ON carts;

DROP TRIGGER IF EXISTS trg_cart_shop_groups_validate_merchant_org ON cart_shop_groups;

DROP TRIGGER IF EXISTS trg_carts_validate_buyer_org ON carts;

DROP FUNCTION IF EXISTS VALIDATE_ORDER_ITEM_MERCHANT_REFS() CASCADE;

DROP FUNCTION IF EXISTS VALIDATE_PAYMENT_BUYER_ORG() CASCADE;

DROP FUNCTION IF EXISTS VALIDATE_INVENTORY_RESERVATION_ITEM() CASCADE;

DROP FUNCTION IF EXISTS VALIDATE_INVENTORY_RESERVATION_ORGS() CASCADE;

DROP FUNCTION IF EXISTS VALIDATE_ORDER_ORGS() CASCADE;

DROP FUNCTION IF EXISTS VALIDATE_CHECKOUT_SESSION_BUYER_CONTEXT() CASCADE;

DROP FUNCTION IF EXISTS VALIDATE_BUYER_CONTEXT(UUID, UUID, UUID) CASCADE;

DROP FUNCTION IF EXISTS VALIDATE_CART_SHOP_GROUP_MERCHANT_ORG() CASCADE;

DROP FUNCTION IF EXISTS VALIDATE_CART_BUYER_ORG() CASCADE;

DROP TABLE IF EXISTS payment_transactions;

DROP TABLE IF EXISTS payments;

DROP TABLE IF EXISTS order_status_histories;

DROP TABLE IF EXISTS inventory_reservation_items;

DROP TABLE IF EXISTS inventory_reservations;

DROP TABLE IF EXISTS order_items;

DROP TABLE IF EXISTS orders;

DROP TABLE IF EXISTS checkout_sessions;

DROP TABLE IF EXISTS cart_items;

DROP TABLE IF EXISTS cart_shop_groups;

DROP TABLE IF EXISTS carts;

-- +goose StatementEnd
