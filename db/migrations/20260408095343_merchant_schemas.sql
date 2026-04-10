-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS categories (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7()
    , organization_id UUID
    , parent_id UUID
    , name TEXT NOT NULL
    , slug TEXT NOT NULL
    , description TEXT
    , sort_order SMALLINT NOT NULL DEFAULT 0
    , is_active BOOL NOT NULL DEFAULT TRUE
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT fk_categories_organization_id FOREIGN KEY (organization_id)
    REFERENCES organizations (id) ON DELETE CASCADE
    , CONSTRAINT fk_categories_parent_id FOREIGN KEY (parent_id)
    REFERENCES categories (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS products (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7()
    , organization_id UUID NOT NULL
    , category_id UUID NOT NULL
    , name TEXT NOT NULL
    , slug TEXT NOT NULL
    , description JSONB NOT NULL
    , status TEXT NOT NULL DEFAULT 'draft'
    , specification JSONB
    , is_featured BOOL NOT NULL DEFAULT FALSE
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT fk_products_organization_id FOREIGN KEY (organization_id)
    REFERENCES organizations (id) ON DELETE CASCADE
    , CONSTRAINT fk_products_category_id FOREIGN KEY (category_id)
    REFERENCES categories (id) ON DELETE CASCADE
    , CONSTRAINT check_products_status CHECK (status IN (
        'active'
        , 'draft'
        , 'archived'
        , 'suspended'
    ))
);

CREATE TABLE IF NOT EXISTS product_variants (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7()
    , product_id UUID NOT NULL
    , organization_id UUID NOT NULL
    , sku TEXT NOT NULL
    , name TEXT NOT NULL
    , price NUMERIC(19, 4) NOT NULL
    , track_inventory BOOL NOT NULL DEFAULT TRUE
    , is_active BOOL NOT NULL DEFAULT FALSE
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT fk_product_variants_product_id FOREIGN KEY (product_id)
    REFERENCES products (id) ON DELETE CASCADE
    , CONSTRAINT fk_product_variants_organization_id FOREIGN KEY (organization_id)
    REFERENCES organizations (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS product_assets (
    id BIGSERIAL PRIMARY KEY
    , product_id UUID NOT NULL
    , product_variant_id UUID NOT NULL
    , asset_key TEXT NOT NULL
    , type TEXT NOT NULL
    , mime_type TEXT NOT NULL
    , alt_text TEXT
    , sort_order SMALLINT NOT NULL DEFAULT 0
    , is_primary BOOL NOT NULL DEFAULT FALSE
    , duration_seconds SMALLINT
    , CONSTRAINT fk_product_assets_product_id FOREIGN KEY (product_id)
    REFERENCES products (id) ON DELETE CASCADE
    , CONSTRAINT fk_product_assets_product_variant_id FOREIGN KEY (product_variant_id)
    REFERENCES product_variants (id) ON DELETE CASCADE
    , CONSTRAINT check_product_assets_type CHECK (type IN (
        'image'
        , 'video'
        , 'document'
    ))
    , CONSTRAINT check_duration_positive
    CHECK (duration_seconds IS NULL OR duration_seconds > 0)
    , CONSTRAINT check_primary_only_on_images
    CHECK (is_primary = FALSE OR type = 'image')
);

CREATE TABLE IF NOT EXISTS warehouses (
    id BIGSERIAL PRIMARY KEY
    , organization_id UUID NOT NULL
    , name TEXT NOT NULL
    , address_id UUID NOT NULL
    , is_active BOOL NOT NULL DEFAULT FALSE
    , CONSTRAINT fk_warehouses_organization_id FOREIGN KEY (organization_id)
    REFERENCES organizations (id) ON DELETE CASCADE
    , CONSTRAINT fk_warehouses_address_id FOREIGN KEY (address_id)
    REFERENCES addresses (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS inventories (
    product_variant_id UUID NOT NULL
    , warehouse_id BIGINT NOT NULL
    , quantity_on_hand INTEGER NOT NULL
    , quantity_reserved INTEGER NOT NULL DEFAULT 0
    , quantity_available INTEGER GENERATED ALWAYS AS
    (quantity_on_hand - quantity_reserved) STORED
    , low_stock_threshold INTEGER
    , is_active BOOL NOT NULL DEFAULT TRUE
    , CONSTRAINT inventories_pkey PRIMARY KEY (product_variant_id, warehouse_id)
    , CONSTRAINT fk_inventories_product_variant_id FOREIGN KEY (product_variant_id)
    REFERENCES product_variants (id) ON DELETE CASCADE
    , CONSTRAINT fk_inventories_warehouse_id FOREIGN KEY (warehouse_id)
    REFERENCES warehouses (id) ON DELETE CASCADE
    , CONSTRAINT check_quantities_non_negative
    CHECK (
        quantity_on_hand >= 0
        AND quantity_reserved >= 0
        AND quantity_reserved <= quantity_on_hand
    )
);

CREATE TABLE IF NOT EXISTS attributes (
    id BIGSERIAL PRIMARY KEY
    , organization_id UUID
    , name TEXT NOT NULL
    , slug TEXT NOT NULL
    , type TEXT NOT NULL
    , CONSTRAINT fk_attributes_organization_id FOREIGN KEY (organization_id)
    REFERENCES organizations (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS attribute_values (
    id BIGSERIAL PRIMARY KEY
    , attribute_id BIGINT NOT NULL
    , organization_id UUID
    , value TEXT NOT NULL
    , label TEXT NOT NULL
    , sort_order SMALLINT NOT NULL DEFAULT 0
    , CONSTRAINT fk_attribute_values_attribute_id FOREIGN KEY (attribute_id)
    REFERENCES attributes (id) ON DELETE CASCADE
    , CONSTRAINT fk_attribute_values_organization_id FOREIGN KEY (organization_id)
    REFERENCES organizations (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS product_variant_attributes (
    product_variant_id UUID NOT NULL
    , attribute_value_id BIGINT NOT NULL
    , CONSTRAINT product_variant_attributes_pkey PRIMARY KEY (
        product_variant_id, attribute_value_id
    )
    , CONSTRAINT fk_product_variant_attributes_product_variant FOREIGN KEY (
        product_variant_id
    )
    REFERENCES product_variants (id) ON DELETE CASCADE
    , CONSTRAINT product_variant_attributes_attribute_value FOREIGN KEY (
        attribute_value_id
    )
    REFERENCES attribute_values (id) ON DELETE CASCADE
);

CREATE TRIGGER trg_products_updated_at
BEFORE UPDATE ON products
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_product_variants_updated_at
BEFORE UPDATE ON product_variants
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_product_variants_updated_at ON product_variants;
DROP TRIGGER IF EXISTS trg_products_updated_at ON products;
DROP TABLE IF EXISTS product_variant_attributes;
DROP TABLE IF EXISTS attribute_values;
DROP TABLE IF EXISTS attributes;
DROP TABLE IF EXISTS inventories;
DROP TABLE IF EXISTS warehouses;
DROP TABLE IF EXISTS product_assets;
DROP TABLE IF EXISTS product_variants;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS categories;
-- +goose StatementEnd
