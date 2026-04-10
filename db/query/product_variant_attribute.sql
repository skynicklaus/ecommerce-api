-- name: AssignAttributeValueToProductVariant :exec
INSERT INTO product_variant_attributes (
    product_variant_id
    , attribute_value_id
) VALUES ($1, $2);
