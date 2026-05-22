-- name: AssignAttributeValueToProductVariant :exec
INSERT INTO
    product_variant_attributes (
        product_variant_id,
        attribute_value_id
    )
VALUES
    ($1, $2);

-- name: ListVariantAttributesByProduct :many
SELECT
    pva.product_variant_id,
    av.id AS attribute_value_id,
    av.value AS attribute_value,
    av.label AS attribute_value_label,
    a.id AS attribute_id,
    a.name AS attribute_name,
    a.slug AS attribute_slug
FROM
    product_variant_attributes pva
    JOIN attribute_values av ON pva.attribute_value_id = av.id
    JOIN attributes a ON av.attribute_id = a.id
    JOIN product_variants pv ON pva.product_variant_id = pv.id
WHERE
    pv.product_id = $1;

-- name: ListVariantAttributesByProductIDs :many
SELECT
    pva.product_variant_id,
    av.id AS attribute_value_id,
    av.value AS attribute_value,
    av.label AS attribute_value_label,
    a.id AS attribute_id,
    a.name AS attribute_name,
    a.slug AS attribute_slug
FROM
    product_variant_attributes pva
    JOIN attribute_values av ON pva.attribute_value_id = av.id
    JOIN attributes a ON av.attribute_id = a.id
    JOIN product_variants pv ON pva.product_variant_id = pv.id
WHERE
    pv.product_id = ANY (sqlc.arg ('product_ids')::UUID []);
