-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS carts (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7(),
    customer_org_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_carts_customer_org_id FOREIGN KEY(customer_org_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS cart_shop_groups (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7(),
    cart_id UUID NOT NULL,
    merchant_org_id UUID NOT NULL,
    is_selected BOOLEAN NOT NULL DEFAULT false,
    subtotal NUMERIC(19, 4) NOT NULL DEFAULT 0.00,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_cart_shop_groups_cart_id FOREIGN KEY(cart_id) REFERENCES carts(id) ON DELETE CASCADE,
    CONSTRAINT fk_cart_shop_groups_merchant_org_id FOREIGN KEY(merchant_org_id) REFERENCES organizations(id) ON DELETE CASCADE,
    CONSTRAINT check_cart_shop_groups_subtotal_non_negative CHECK (subtotal >= 0)
);

CREATE TABLE IF NOT EXISTS cart_items (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7(),
    cart_shop_group_id UUID NOT NULL,
    product_variant_id UUID NOT NULL,
    quantity SMALLINT NOT NULL,
    unit_price NUMERIC(19, 4) NOT NULL,
    is_selected BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_cart_items_cart_shop_group_id FOREIGN KEY(cart_shop_group_id) REFERENCES cart_shop_groups(id) ON DELETE CASCADE,
    CONSTRAINT fk_cart_items_product_variant_id FOREIGN KEY(product_variant_id) REFERENCES product_variants(id),
    CONSTRAINT check_cart_items_quantity_positive CHECK (quantity > 0),
    CONSTRAINT check_cart_items_unit_price_non_negative CHECK (unit_price >= 0)
);

CREATE
OR REPLACE FUNCTION validate_cart_customer_org() RETURNS TRIGGER AS
$$
BEGIN
    IF NOT EXISTS (
        SELECT
            1
        FROM
            organizations
        WHERE
            id = NEW.customer_org_id
            AND capability = 'buyer'
    ) THEN
        RAISE EXCEPTION 'cart customer_org_id must reference a buyer-capable organization';
    END IF;

    RETURN NEW;
END;
$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION validate_cart_shop_group_merchant_org() RETURNS TRIGGER AS
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
    ) THEN
        RAISE EXCEPTION 'cart merchant_org_id must reference a seller-capable organization';
    END IF;

    RETURN NEW;
END;
$$
LANGUAGE plpgsql;

CREATE TRIGGER trg_carts_validate_customer_org BEFORE
INSERT
    OR UPDATE OF customer_org_id ON carts FOR EACH ROW EXECUTE FUNCTION validate_cart_customer_org();

CREATE TRIGGER trg_cart_shop_groups_validate_merchant_org BEFORE
INSERT
    OR UPDATE OF merchant_org_id ON cart_shop_groups FOR EACH ROW EXECUTE FUNCTION validate_cart_shop_group_merchant_org();

CREATE TRIGGER trg_carts_updated_at BEFORE
UPDATE
    ON carts FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_cart_shop_groups_updated_at BEFORE
UPDATE
    ON cart_shop_groups FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_cart_items_updated_at BEFORE
UPDATE
    ON cart_items FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_cart_items_updated_at ON cart_items;

DROP TRIGGER IF EXISTS trg_cart_shop_groups_updated_at ON cart_shop_groups;

DROP TRIGGER IF EXISTS trg_carts_updated_at ON carts;

DROP TRIGGER IF EXISTS trg_cart_shop_groups_validate_merchant_org ON cart_shop_groups;

DROP TRIGGER IF EXISTS trg_carts_validate_customer_org ON carts;

DROP FUNCTION IF EXISTS validate_cart_shop_group_merchant_org() CASCADE;

DROP FUNCTION IF EXISTS validate_cart_customer_org() CASCADE;

DROP TABLE IF EXISTS cart_items;

DROP TABLE IF EXISTS cart_shop_groups;

DROP TABLE IF EXISTS carts;

-- +goose StatementEnd
