-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS categories (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7(),
    organization_id UUID,
    parent_id UUID,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT,
    sort_order SMALLINT NOT NULL DEFAULT 0,
    is_active BOOL NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_categories_organization_id FOREIGN KEY (organization_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_categories_parent_id FOREIGN KEY (parent_id) REFERENCES categories (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS products (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7(),
    organization_id UUID NOT NULL,
    category_id UUID NOT NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description JSONB NOT NULL,
    "status" TEXT NOT NULL DEFAULT 'draft',
    specification JSONB,
    is_featured BOOL NOT NULL DEFAULT FALSE,
    idempotency_key TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_products_organization_id FOREIGN KEY (organization_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_products_category_id FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE CASCADE,
    CONSTRAINT check_products_status CHECK (
        "status" IN (
            'active',
            'draft',
            'archived',
            'suspended'
        )
    )
);

CREATE TABLE IF NOT EXISTS product_variants (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7(),
    product_id UUID NOT NULL,
    organization_id UUID NOT NULL,
    sku TEXT NOT NULL,
    name TEXT NOT NULL,
    price NUMERIC(19, 4) NOT NULL,
    track_inventory BOOL NOT NULL DEFAULT TRUE,
    is_active BOOL NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_product_variants_product_id FOREIGN KEY (product_id) REFERENCES products (id) ON DELETE CASCADE,
    CONSTRAINT fk_product_variants_organization_id FOREIGN KEY (organization_id) REFERENCES organizations (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS product_assets (
    id BIGSERIAL PRIMARY KEY,
    product_id UUID NOT NULL,
    product_variant_id UUID,
    asset_key TEXT NOT NULL,
    "type" TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    alt_text TEXT,
    sort_order SMALLINT NOT NULL DEFAULT 0,
    is_primary BOOL NOT NULL DEFAULT FALSE,
    duration_seconds SMALLINT,
    CONSTRAINT fk_product_assets_product_id FOREIGN KEY (product_id) REFERENCES products (id) ON DELETE CASCADE,
    CONSTRAINT fk_product_assets_product_variant_id FOREIGN KEY (product_variant_id) REFERENCES product_variants (id) ON DELETE CASCADE,
    CONSTRAINT check_product_assets_type CHECK (
        "type" IN (
            'image',
            'video',
            'document'
        )
    ),
    CONSTRAINT check_duration_positive CHECK (
        duration_seconds IS NULL
        OR duration_seconds > 0
    ),
    CONSTRAINT check_primary_only_on_images CHECK (
        is_primary = FALSE
        OR "type" = 'image'
    )
);

CREATE TABLE IF NOT EXISTS warehouses (
    id BIGSERIAL PRIMARY KEY,
    organization_id UUID NOT NULL,
    name TEXT NOT NULL,
    address_id UUID NOT NULL,
    is_active BOOL NOT NULL DEFAULT FALSE,
    CONSTRAINT fk_warehouses_organization_id FOREIGN KEY (organization_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_warehouses_address_id FOREIGN KEY (address_id) REFERENCES addresses (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS inventories (
    product_variant_id UUID NOT NULL,
    warehouse_id BIGINT NOT NULL,
    quantity_on_hand INTEGER NOT NULL,
    quantity_reserved INTEGER NOT NULL DEFAULT 0,
    quantity_available INTEGER GENERATED ALWAYS AS (quantity_on_hand - quantity_reserved) STORED,
    low_stock_threshold INTEGER,
    is_active BOOL NOT NULL DEFAULT TRUE,
    CONSTRAINT inventories_pkey PRIMARY KEY (product_variant_id, warehouse_id),
    CONSTRAINT fk_inventories_product_variant_id FOREIGN KEY (product_variant_id) REFERENCES product_variants (id) ON DELETE CASCADE,
    CONSTRAINT fk_inventories_warehouse_id FOREIGN KEY (warehouse_id) REFERENCES warehouses (id) ON DELETE CASCADE,
    CONSTRAINT check_quantities_non_negative CHECK (
        quantity_on_hand >= 0
        AND quantity_reserved >= 0
        AND quantity_reserved <= quantity_on_hand
    )
);

CREATE TABLE IF NOT EXISTS attributes (
    id BIGSERIAL PRIMARY KEY,
    organization_id UUID,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    "type" TEXT NOT NULL,
    CONSTRAINT fk_attributes_organization_id FOREIGN KEY (organization_id) REFERENCES organizations (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS attribute_values (
    id BIGSERIAL PRIMARY KEY,
    attribute_id BIGINT NOT NULL,
    organization_id UUID,
    value TEXT NOT NULL,
    label TEXT NOT NULL,
    sort_order SMALLINT NOT NULL DEFAULT 0,
    CONSTRAINT fk_attribute_values_attribute_id FOREIGN KEY (attribute_id) REFERENCES attributes (id) ON DELETE CASCADE,
    CONSTRAINT fk_attribute_values_organization_id FOREIGN KEY (organization_id) REFERENCES organizations (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS product_variant_attributes (
    product_variant_id UUID NOT NULL,
    attribute_value_id BIGINT NOT NULL,
    CONSTRAINT product_variant_attributes_pkey PRIMARY KEY (
        product_variant_id,
        attribute_value_id
    ),
    CONSTRAINT fk_product_variant_attributes_product_variant FOREIGN KEY (product_variant_id) REFERENCES product_variants (id) ON DELETE CASCADE,
    CONSTRAINT product_variant_attributes_attribute_value FOREIGN KEY (attribute_value_id) REFERENCES attribute_values (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS product_search_documents (
    product_id UUID PRIMARY KEY,
    search_vector tsvector NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_product_search_documents_product_id FOREIGN KEY (product_id) REFERENCES products (id) ON DELETE CASCADE
);

CREATE
OR REPLACE FUNCTION rebuild_product_search_document(p_product_id uuid) RETURNS void AS
$$
BEGIN
    INSERT INTO
        product_search_documents (
            product_id,
            search_vector,
            updated_at
        )
    SELECT
        p.id,
        setweight(to_tsvector('simple', COALESCE(p.name, '')), 'A') ||
        setweight(to_tsvector('simple', COALESCE(variant_doc.names, '')), 'A') ||
        setweight(to_tsvector('simple', COALESCE(variant_doc.skus, '')), 'A') ||
        setweight(to_tsvector('simple', COALESCE(c.name, '')), 'B') ||
        setweight(
            jsonb_to_tsvector(
                'simple',
                COALESCE(p.description, '{}'::jsonb),
                '["string"]'
            ),
            'B'
        ) ||
        setweight(to_tsvector('simple', COALESCE(attribute_doc.values, '')), 'B') ||
        setweight(to_tsvector('simple', COALESCE(attribute_doc.labels, '')), 'B') ||
        setweight(
            jsonb_to_tsvector(
                'simple',
                COALESCE(p.specification, '{}'::jsonb),
                '["string", "key"]'
            ),
            'C'
        ),
        NOW()
    FROM
        products p
        JOIN categories c ON c.id = p.category_id
        LEFT JOIN LATERAL (
            SELECT
                string_agg(DISTINCT pv.name, ' ') AS names,
                string_agg(DISTINCT pv.sku, ' ') AS skus
            FROM
                product_variants pv
            WHERE
                pv.product_id = p.id
                AND pv.is_active = TRUE
        ) variant_doc ON TRUE
        LEFT JOIN LATERAL (
            SELECT
                string_agg(DISTINCT av.value, ' ') AS values,
                string_agg(DISTINCT av.label, ' ') AS labels
            FROM
                product_variants pv
                JOIN product_variant_attributes pva ON pva.product_variant_id = pv.id
                JOIN attribute_values av ON av.id = pva.attribute_value_id
            WHERE
                pv.product_id = p.id
                AND pv.is_active = TRUE
        ) attribute_doc ON TRUE
    WHERE
        p.id = p_product_id ON CONFLICT (product_id) DO
    UPDATE
    SET
        search_vector = EXCLUDED.search_vector,
        updated_at = NOW();
END;
$$
LANGUAGE plpgsql SECURITY DEFINER
SET
    search_path = pg_catalog,
    public;

CREATE
OR REPLACE FUNCTION skip_product_search_document_refresh() RETURNS bool AS
$$
BEGIN
    RETURN COALESCE(NULLIF(current_setting('app.skip_search_doc_refresh', TRUE), ''), 'false')::bool;
END;
$$
LANGUAGE plpgsql STABLE;

CREATE
OR REPLACE FUNCTION refresh_product_search_document_from_product() RETURNS TRIGGER AS
$$
BEGIN
    IF skip_product_search_document_refresh() THEN
        RETURN NEW;
    END IF;

    PERFORM rebuild_product_search_document(NEW.id);
    RETURN NEW;
END;
$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION refresh_product_search_document_from_category() RETURNS TRIGGER AS
$$
DECLARE
    affected_product_id uuid;
BEGIN
    IF skip_product_search_document_refresh() THEN
        RETURN NEW;
    END IF;

    FOR affected_product_id IN
        SELECT
            id
        FROM
            products
        WHERE
            category_id = NEW.id
    LOOP
        PERFORM rebuild_product_search_document(affected_product_id);
    END LOOP;

    RETURN NEW;
END;
$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION refresh_product_search_document_from_variant() RETURNS TRIGGER AS
$$
BEGIN
    IF skip_product_search_document_refresh() THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;

    IF TG_OP = 'DELETE' THEN
        PERFORM rebuild_product_search_document(OLD.product_id);
        RETURN OLD;
    END IF;

    PERFORM rebuild_product_search_document(NEW.product_id);
    RETURN NEW;
END;
$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION refresh_product_search_document_from_variant_attribute() RETURNS TRIGGER AS
$$
DECLARE
    affected_product_id uuid;
BEGIN
    IF skip_product_search_document_refresh() THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;

    SELECT
        product_id
    INTO
        affected_product_id
    FROM
        product_variants
    WHERE
        id = COALESCE(NEW.product_variant_id, OLD.product_variant_id);

    IF affected_product_id IS NOT NULL THEN
        PERFORM rebuild_product_search_document(affected_product_id);
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION refresh_product_search_document_from_attribute_value() RETURNS TRIGGER AS
$$
DECLARE
    affected_product_id uuid;
BEGIN
    IF skip_product_search_document_refresh() THEN
        RETURN NEW;
    END IF;

    FOR affected_product_id IN
        SELECT DISTINCT
            pv.product_id
        FROM
            product_variant_attributes pva
            JOIN product_variants pv ON pv.id = pva.product_variant_id
        WHERE
            pva.attribute_value_id = NEW.id
    LOOP
        PERFORM rebuild_product_search_document(affected_product_id);
    END LOOP;

    RETURN NEW;
END;
$$
LANGUAGE plpgsql;

CREATE TRIGGER trg_products_refresh_search_document AFTER
INSERT
    OR UPDATE OF name,
    description,
    specification,
    category_id ON products FOR EACH ROW EXECUTE FUNCTION refresh_product_search_document_from_product();

CREATE TRIGGER trg_categories_refresh_product_search_documents AFTER
UPDATE OF name ON categories FOR EACH ROW EXECUTE FUNCTION refresh_product_search_document_from_category();

CREATE TRIGGER trg_product_variants_refresh_search_document AFTER
INSERT
    OR UPDATE OF name,
    sku,
    is_active
    OR DELETE ON product_variants FOR EACH ROW EXECUTE FUNCTION refresh_product_search_document_from_variant();

CREATE TRIGGER trg_product_variant_attributes_refresh_search_document AFTER
INSERT
    OR DELETE ON product_variant_attributes FOR EACH ROW EXECUTE FUNCTION refresh_product_search_document_from_variant_attribute();

CREATE TRIGGER trg_attribute_values_refresh_product_search_documents AFTER
UPDATE OF value,
label ON attribute_values FOR EACH ROW EXECUTE FUNCTION refresh_product_search_document_from_attribute_value();

CREATE TRIGGER trg_products_updated_at BEFORE
UPDATE
    ON products FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_product_variants_updated_at BEFORE
UPDATE
    ON product_variants FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_product_variants_updated_at ON product_variants;

DROP TRIGGER IF EXISTS trg_products_updated_at ON products;

DROP TRIGGER IF EXISTS trg_attribute_values_refresh_product_search_documents ON attribute_values;

DROP TRIGGER IF EXISTS trg_product_variant_attributes_refresh_search_document ON product_variant_attributes;

DROP TRIGGER IF EXISTS trg_product_variants_refresh_search_document ON product_variants;

DROP TRIGGER IF EXISTS trg_categories_refresh_product_search_documents ON categories;

DROP TRIGGER IF EXISTS trg_products_refresh_search_document ON products;

DROP FUNCTION IF EXISTS refresh_product_search_document_from_attribute_value();

DROP FUNCTION IF EXISTS refresh_product_search_document_from_variant_attribute();

DROP FUNCTION IF EXISTS refresh_product_search_document_from_variant();

DROP FUNCTION IF EXISTS refresh_product_search_document_from_category();

DROP FUNCTION IF EXISTS refresh_product_search_document_from_product();

DROP FUNCTION IF EXISTS skip_product_search_document_refresh();

DROP FUNCTION IF EXISTS rebuild_product_search_document(uuid);

DROP TABLE IF EXISTS product_search_documents;

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
