//go:build integration

package db_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestSearchProducts(t *testing.T) {
	org := createRandomOrganization(t)
	category := createRandomCategory(t)
	term := "searchterm" + uuid.NewString()[:8]

	nameMatch := createSearchProduct(t, org, category, "Premium "+term+" Sofa", "comfortable living room seating")
	descriptionMatch := createSearchProduct(t, org, category, "Premium Sofa", "comfortable "+term+" living room seating")
	draftMatch := createSearchProduct(t, org, category, "Draft "+term+" Sofa", "draft should be excluded")
	irrelevant := createSearchProduct(t, org, category, "Unrelated Product", "does not include the unique token")

	activateProduct(t, nameMatch)
	activateProduct(t, descriptionMatch)
	activateProduct(t, irrelevant)
	upsertSearchDocument(t, nameMatch.ID)
	upsertSearchDocument(t, descriptionMatch.ID)
	upsertSearchDocument(t, draftMatch.ID)
	upsertSearchDocument(t, irrelevant.ID)

	rows, err := testStore.SearchProducts(t.Context(), firstSearchPageParams(term, 10))
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, nameMatch.ID, rows[0].ID)
	require.Equal(t, descriptionMatch.ID, rows[1].ID)
	require.Greater(t, rows[0].Rank, rows[1].Rank)

	ids := make(map[uuid.UUID]struct{}, len(rows))
	for _, row := range rows {
		ids[row.ID] = struct{}{}
		require.Equal(t, string(util.ProductStatusActive), row.Status)
	}
	_, foundDraft := ids[draftMatch.ID]
	require.False(t, foundDraft)
	_, foundIrrelevant := ids[irrelevant.ID]
	require.False(t, foundIrrelevant)
}

func TestProductSearchDocumentRefreshTriggers(t *testing.T) {
	org := createRandomOrganization(t)
	category := createRandomCategory(t)
	product := createSearchProduct(t, org, category, "Initial Search Refresh Product", "initial description")
	variant, err := testStore.CreateProductVariant(t.Context(), db.CreateProductVariantParams{
		ProductID:      product.ID,
		OrganizationID: org.ID,
		Sku:            "initial-refresh-sku-" + uuid.NewString()[:8],
		Name:           "Initial Refresh Variant",
		Price:          util.GetRandomPrice(),
	})
	require.NoError(t, err)
	_, err = testPool.Exec(t.Context(), "UPDATE product_variants SET is_active = TRUE WHERE id = $1", variant.ID)
	require.NoError(t, err)
	attribute := createRandomAttributeWithOrg(t, &org)
	attributeValue, err := testStore.CreateAttributeValue(t.Context(), db.CreateAttributeValueParams{
		AttributeID:    attribute.ID,
		OrganizationID: attribute.OrganizationID,
		Value:          "initial-refresh-value",
		Label:          "Initial Refresh Label",
		SortOrder:      1,
	})
	require.NoError(t, err)
	err = testStore.AssignAttributeValueToProductVariant(t.Context(), db.AssignAttributeValueToProductVariantParams{
		ProductVariantID: variant.ID,
		AttributeValueID: attributeValue.ID,
	})
	require.NoError(t, err)
	activateProduct(t, product)

	productTerm := "productrefresh" + uuid.NewString()[:8]
	_, err = testPool.Exec(
		t.Context(),
		`UPDATE products SET name = $1, description = $2, specification = $3 WHERE id = $4`,
		"Updated "+productTerm,
		[]byte(fmt.Sprintf(`{"text":%q}`, productTerm)),
		[]byte(fmt.Sprintf(`{"search_key":%q}`, productTerm)),
		product.ID,
	)
	require.NoError(t, err)
	assertSearchFindsProduct(t, productTerm, product.ID)

	categoryTerm := "categoryrefresh" + uuid.NewString()[:8]
	_, err = testPool.Exec(t.Context(), "UPDATE categories SET name = $1 WHERE id = $2", categoryTerm, category.ID)
	require.NoError(t, err)
	assertSearchFindsProduct(t, categoryTerm, product.ID)

	variantTerm := "variantrefresh" + uuid.NewString()[:8]
	_, err = testPool.Exec(
		t.Context(),
		"UPDATE product_variants SET name = $1, sku = $2 WHERE id = $3",
		variantTerm,
		variantTerm,
		variant.ID,
	)
	require.NoError(t, err)
	assertSearchFindsProduct(t, variantTerm, product.ID)

	attributeTerm := "attributerefresh" + uuid.NewString()[:8]
	_, err = testPool.Exec(
		t.Context(),
		"UPDATE attribute_values SET value = $1, label = $2 WHERE id = $3",
		attributeTerm,
		attributeTerm,
		attributeValue.ID,
	)
	require.NoError(t, err)
	assertSearchFindsProduct(t, attributeTerm, product.ID)
}

func TestSearchProductsPagination(t *testing.T) {
	org := createRandomOrganization(t)
	category := createRandomCategory(t)
	term := "pageable" + uuid.NewString()[:8]

	products := make([]db.Product, 3)
	for i := range products {
		products[i] = createSearchProduct(t, org, category, fmt.Sprintf("Product %d %s", i, term), "same searchable text")
		activateProduct(t, products[i])
		upsertSearchDocument(t, products[i].ID)
	}

	firstPage, err := testStore.SearchProducts(t.Context(), firstSearchPageParams(term, 2))
	require.NoError(t, err)
	require.Len(t, firstPage, 2)

	last := firstPage[len(firstPage)-1]
	secondPage, err := testStore.SearchProducts(t.Context(), db.SearchProductsParams{
		AfterRank:      last.Rank,
		AfterCreatedAt: last.CreatedAt,
		AfterID:        last.ID,
		Query:          term,
		PageLimit:      2,
	})
	require.NoError(t, err)
	require.Len(t, secondPage, 1)

	seen := map[uuid.UUID]struct{}{}
	for _, row := range append(firstPage, secondPage...) {
		_, duplicate := seen[row.ID]
		require.False(t, duplicate, "product %s returned on multiple pages", row.ID)
		seen[row.ID] = struct{}{}
	}
	require.Len(t, seen, 3)
}

func TestSearchProductsWithSearchDocument(t *testing.T) {
	org := createRandomOrganization(t)
	category := createRandomCategory(t)
	categoryTerm := "categoryterm" + uuid.NewString()[:8]
	variantTerm := "variantterm" + uuid.NewString()[:8]
	skuTerm := "skuterm" + uuid.NewString()[:8]
	attributeTerm := "attrterm" + uuid.NewString()[:8]
	labelTerm := "labelterm" + uuid.NewString()[:8]

	category.Name = categoryTerm
	_, err := testPool.Exec(t.Context(), "UPDATE categories SET name = $1 WHERE id = $2", category.Name, category.ID)
	require.NoError(t, err)

	product := createSearchProduct(t, org, category, "Document Product", "plain description")
	variant, err := testStore.CreateProductVariant(t.Context(), db.CreateProductVariantParams{
		ProductID:      product.ID,
		OrganizationID: org.ID,
		Sku:            skuTerm,
		Name:           variantTerm,
		Price:          util.GetRandomPrice(),
	})
	require.NoError(t, err)
	_, err = testPool.Exec(t.Context(), "UPDATE product_variants SET is_active = TRUE WHERE id = $1", variant.ID)
	require.NoError(t, err)
	attribute := createRandomAttributeWithOrg(t, &org)
	attributeValue, err := testStore.CreateAttributeValue(t.Context(), db.CreateAttributeValueParams{
		AttributeID:    attribute.ID,
		OrganizationID: attribute.OrganizationID,
		Value:          attributeTerm,
		Label:          labelTerm,
		SortOrder:      1,
	})
	require.NoError(t, err)
	err = testStore.AssignAttributeValueToProductVariant(t.Context(), db.AssignAttributeValueToProductVariantParams{
		ProductVariantID: variant.ID,
		AttributeValueID: attributeValue.ID,
	})
	require.NoError(t, err)
	activateProduct(t, product)
	upsertSearchDocument(t, product.ID)

	for _, query := range []string{categoryTerm, variantTerm, skuTerm, attributeTerm, labelTerm} {
		rows, searchErr := testStore.SearchProducts(t.Context(), firstSearchPageParams(query, 10))
		require.NoError(t, searchErr)
		require.NotEmpty(t, rows, "query %q should match product search document", query)
		require.Equal(t, product.ID, rows[0].ID)
	}
}

func TestSearchProductsExcludesInactiveVariantDocumentTerms(t *testing.T) {
	org := createRandomOrganization(t)
	category := createRandomCategory(t)
	variantTerm := "inactivevariant" + uuid.NewString()[:8]
	attributeTerm := "inactiveattribute" + uuid.NewString()[:8]

	product := createSearchProduct(t, org, category, "Active Product Without Variant Term", "plain description")
	variant, err := testStore.CreateProductVariant(t.Context(), db.CreateProductVariantParams{
		ProductID:      product.ID,
		OrganizationID: org.ID,
		Sku:            variantTerm,
		Name:           variantTerm,
		Price:          util.GetRandomPrice(),
	})
	require.NoError(t, err)
	_, err = testPool.Exec(t.Context(), "UPDATE product_variants SET is_active = TRUE WHERE id = $1", variant.ID)
	require.NoError(t, err)
	attribute := createRandomAttributeWithOrg(t, &org)
	attributeValue, err := testStore.CreateAttributeValue(t.Context(), db.CreateAttributeValueParams{
		AttributeID:    attribute.ID,
		OrganizationID: attribute.OrganizationID,
		Value:          attributeTerm,
		Label:          attributeTerm,
		SortOrder:      1,
	})
	require.NoError(t, err)
	err = testStore.AssignAttributeValueToProductVariant(t.Context(), db.AssignAttributeValueToProductVariantParams{
		ProductVariantID: variant.ID,
		AttributeValueID: attributeValue.ID,
	})
	require.NoError(t, err)
	activateProduct(t, product)
	upsertSearchDocument(t, product.ID)
	assertSearchFindsProduct(t, variantTerm, product.ID)
	assertSearchFindsProduct(t, attributeTerm, product.ID)

	err = testStore.DeleteProductVariant(t.Context(), db.DeleteProductVariantParams{
		ID:             variant.ID,
		OrganizationID: org.ID,
	})
	require.NoError(t, err)

	assertSearchDoesNotFindProduct(t, variantTerm, product.ID)
	assertSearchDoesNotFindProduct(t, attributeTerm, product.ID)
}

func assertSearchFindsProduct(t *testing.T, query string, productID uuid.UUID) {
	t.Helper()

	rows, err := testStore.SearchProducts(t.Context(), firstSearchPageParams(query, 10))
	require.NoError(t, err)
	for _, row := range rows {
		if row.ID == productID {
			return
		}
	}
	require.Failf(t, "product not found in search results", "query %q did not return product %s", query, productID)
}

func assertSearchDoesNotFindProduct(t *testing.T, query string, productID uuid.UUID) {
	t.Helper()

	rows, err := testStore.SearchProducts(t.Context(), firstSearchPageParams(query, 10))
	require.NoError(t, err)
	for _, row := range rows {
		require.NotEqual(t, productID, row.ID, "query %q should not return product %s", query, productID)
	}
}

func firstSearchPageParams(query string, limit int32) db.SearchProductsParams {
	return db.SearchProductsParams{
		AfterRank:      math.Inf(1),
		AfterCreatedAt: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC),
		AfterID:        uuid.Max,
		Query:          query,
		PageLimit:      limit,
	}
}

func createSearchProduct(
	t *testing.T,
	org db.Organization,
	category db.Category,
	name string,
	description string,
) db.Product {
	t.Helper()

	product, err := testStore.CreateProduct(t.Context(), db.CreateProductParams{
		OrganizationID: org.ID,
		CategoryID:     category.ID,
		Name:           name,
		Slug:           fmt.Sprintf("%s-%s", org.Slug, uuid.NewString()),
		Description:    []byte(fmt.Sprintf(`{"text":%q}`, description)),
		Specification:  []byte(`{"material":"wood"}`),
	})
	require.NoError(t, err)
	return product
}

func upsertSearchDocument(t *testing.T, productID uuid.UUID) {
	t.Helper()

	err := testStore.UpsertProductSearchDocument(t.Context(), productID)
	require.NoError(t, err)
}

func activateProduct(t *testing.T, product db.Product) {
	t.Helper()

	_, err := testStore.UpdateProductStatus(t.Context(), db.UpdateProductStatusParams{
		ID:             product.ID,
		OrganizationID: product.OrganizationID,
		Status:         string(util.ProductStatusActive),
	})
	require.NoError(t, err)
}
