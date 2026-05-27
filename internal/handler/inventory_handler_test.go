//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestUpsertInventoryHandler_Integration(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)

	createdWarehouse := createWarehouseFixtureForHandler(t, tc)

	threshold := int32(5)
	inventoryReq := UpsertInventoryRequest{
		ProductVariantID:  tc.productVariant.ID,
		WarehouseID:       createdWarehouse.ID,
		QuantityOnHand:    25,
		LowStockThreshold: &threshold,
		IsActive:          new(true),
	}
	inventoryBody, err := json.Marshal(inventoryReq)
	require.NoError(t, err)

	inventoryHTTPReq := requestWithOrganization(
		httptest.NewRequest(http.MethodPut, "/v1/merchant/inventory", bytes.NewReader(inventoryBody)),
		tc.organization,
	)
	inventoryRec := httptest.NewRecorder()
	err = tc.handler.UpsertInventory(inventoryRec, inventoryHTTPReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, inventoryRec.Code)

	var inventory InventoryResponse
	require.NoError(t, json.Unmarshal(inventoryRec.Body.Bytes(), &inventory))
	require.Equal(t, int32(25), inventory.QuantityOnHand)
	require.Equal(t, threshold, *inventory.LowStockThreshold)
	require.Equal(t, tc.productVariant.ID, inventory.ProductVariantID)

	inventoryReq.QuantityOnHand = 35
	inventoryReq.LowStockThreshold = nil
	inventoryBody, err = json.Marshal(inventoryReq)
	require.NoError(t, err)

	inventoryHTTPReq = requestWithOrganization(
		httptest.NewRequest(http.MethodPut, "/v1/merchant/inventory", bytes.NewReader(inventoryBody)),
		tc.organization,
	)
	inventoryRec = httptest.NewRecorder()
	err = tc.handler.UpsertInventory(inventoryRec, inventoryHTTPReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, inventoryRec.Code)

	inventory = InventoryResponse{}
	require.NoError(t, json.Unmarshal(inventoryRec.Body.Bytes(), &inventory))
	require.Equal(t, int32(35), inventory.QuantityOnHand)
	require.Nil(t, inventory.LowStockThreshold)
}

func TestUpsertInventoryHandler_CrossOrgVariantNotFound(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)
	otherTC := setupWarehouseInventoryHandlerTest(t)
	createdWarehouse := createWarehouseFixtureForHandler(t, tc)

	inventoryReq := UpsertInventoryRequest{
		ProductVariantID: otherTC.productVariant.ID,
		WarehouseID:      createdWarehouse.ID,
		QuantityOnHand:   25,
		IsActive:         new(true),
	}
	inventoryBody, err := json.Marshal(inventoryReq)
	require.NoError(t, err)

	inventoryHTTPReq := requestWithOrganization(
		httptest.NewRequest(http.MethodPut, "/v1/merchant/inventory", bytes.NewReader(inventoryBody)),
		tc.organization,
	)
	inventoryRec := httptest.NewRecorder()
	err = tc.handler.UpsertInventory(inventoryRec, inventoryHTTPReq)
	requireAPIErrorStatus(t, err, http.StatusNotFound)
}

func TestUpsertInventoryHandler_CrossOrgWarehouseNotFound(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)
	otherTC := setupWarehouseInventoryHandlerTest(t)
	otherWarehouse := createWarehouseFixtureForHandler(t, otherTC)

	inventoryReq := UpsertInventoryRequest{
		ProductVariantID: tc.productVariant.ID,
		WarehouseID:      otherWarehouse.ID,
		QuantityOnHand:   25,
		IsActive:         new(true),
	}
	inventoryBody, err := json.Marshal(inventoryReq)
	require.NoError(t, err)

	inventoryHTTPReq := requestWithOrganization(
		httptest.NewRequest(http.MethodPut, "/v1/merchant/inventory", bytes.NewReader(inventoryBody)),
		tc.organization,
	)
	inventoryRec := httptest.NewRecorder()
	err = tc.handler.UpsertInventory(inventoryRec, inventoryHTTPReq)
	requireAPIErrorStatus(t, err, http.StatusNotFound)
}

func TestUpsertInventoryHandler_ValidationError(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)

	inventoryReq := UpsertInventoryRequest{
		ProductVariantID: tc.productVariant.ID,
		WarehouseID:      0,
		QuantityOnHand:   -1,
		IsActive:         nil,
	}
	inventoryBody, err := json.Marshal(inventoryReq)
	require.NoError(t, err)

	inventoryHTTPReq := requestWithOrganization(
		httptest.NewRequest(http.MethodPut, "/v1/merchant/inventory", bytes.NewReader(inventoryBody)),
		tc.organization,
	)
	inventoryRec := httptest.NewRecorder()
	err = tc.handler.UpsertInventory(inventoryRec, inventoryHTTPReq)
	requireAPIErrorStatus(t, err, http.StatusUnprocessableEntity)
}

func TestListInventoryHandler_ByVariantID(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)
	createdWarehouse := createWarehouseFixtureForHandler(t, tc)
	upsertInventoryForHandler(t, tc, createdWarehouse.ID, tc.productVariant.ID, 25)

	listReq := requestWithOrganization(
		httptest.NewRequest(
			http.MethodGet,
			"/v1/merchant/inventory?variantId="+tc.productVariant.ID.String(),
			nil,
		),
		tc.organization,
	)
	listRec := httptest.NewRecorder()
	err := tc.handler.ListInventory(listRec, listReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listRec.Code)

	var inventoryList ListInventoryResponse
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &inventoryList))
	require.Len(t, inventoryList.Data, 1)
	require.Equal(t, tc.productVariant.ID, inventoryList.Data[0].ProductVariantID)
	require.Equal(t, createdWarehouse.ID, inventoryList.Data[0].WarehouseID)
}

func TestListInventoryHandler_InvalidVariantID(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)

	listReq := requestWithOrganization(
		httptest.NewRequest(http.MethodGet, "/v1/merchant/inventory?variantId=not-a-uuid", nil),
		tc.organization,
	)
	listRec := httptest.NewRecorder()
	err := tc.handler.ListInventory(listRec, listReq)
	requireAPIErrorStatus(t, err, http.StatusBadRequest)
}

func TestListInventoryHandler_ByOrganization(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)
	createdWarehouse := createWarehouseFixtureForHandler(t, tc)
	upsertInventoryForHandler(t, tc, createdWarehouse.ID, tc.productVariant.ID, 25)

	listReq := requestWithOrganization(
		httptest.NewRequest(http.MethodGet, "/v1/merchant/inventory", nil),
		tc.organization,
	)
	listRec := httptest.NewRecorder()
	err := tc.handler.ListInventory(listRec, listReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listRec.Code)

	var inventoryList ListInventoryResponse
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &inventoryList))
	require.Len(t, inventoryList.Data, 1)
	require.Equal(t, tc.productVariant.ID, inventoryList.Data[0].ProductVariantID)
	require.Equal(t, createdWarehouse.ID, inventoryList.Data[0].WarehouseID)
	require.Equal(t, tc.productVariant.ProductID, inventoryList.Data[0].ProductID)
	require.NotEmpty(t, inventoryList.Data[0].ProductName)
	require.NotEmpty(t, inventoryList.Data[0].VariantSku)
	require.NotEmpty(t, inventoryList.Data[0].VariantName)
	require.Equal(t, createdWarehouse.Name, inventoryList.Data[0].WarehouseName)
}

func TestListInventoryHandler_ByOrganizationPagination(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)
	firstWarehouse := createWarehouseFixtureForHandlerWithName(t, tc, "A Inventory Warehouse")
	secondWarehouse := createWarehouseFixtureForHandlerWithName(t, tc, "B Inventory Warehouse")
	upsertInventoryForHandler(t, tc, firstWarehouse.ID, tc.productVariant.ID, 25)
	upsertInventoryForHandler(t, tc, secondWarehouse.ID, tc.productVariant.ID, 15)

	firstPageReq := requestWithOrganization(
		httptest.NewRequest(http.MethodGet, "/v1/merchant/inventory?limit=1", nil),
		tc.organization,
	)
	firstPageRec := httptest.NewRecorder()
	err := tc.handler.ListInventory(firstPageRec, firstPageReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, firstPageRec.Code)

	var firstPage ListInventoryResponse
	require.NoError(t, json.Unmarshal(firstPageRec.Body.Bytes(), &firstPage))
	require.Len(t, firstPage.Data, 1)
	require.Equal(t, firstWarehouse.ID, firstPage.Data[0].WarehouseID)
	require.NotNil(t, firstPage.NextCursor)

	secondPageReq := requestWithOrganization(
		httptest.NewRequest(
			http.MethodGet,
			"/v1/merchant/inventory?limit=1&cursor="+*firstPage.NextCursor,
			nil,
		),
		tc.organization,
	)
	secondPageRec := httptest.NewRecorder()
	err = tc.handler.ListInventory(secondPageRec, secondPageReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, secondPageRec.Code)

	var secondPage ListInventoryResponse
	require.NoError(t, json.Unmarshal(secondPageRec.Body.Bytes(), &secondPage))
	require.Len(t, secondPage.Data, 1)
	require.Equal(t, secondWarehouse.ID, secondPage.Data[0].WarehouseID)
	require.Nil(t, secondPage.NextCursor)
}

func TestListInventoryHandler_InvalidCursor(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)

	listReq := requestWithOrganization(
		httptest.NewRequest(http.MethodGet, "/v1/merchant/inventory?cursor=not-a-cursor", nil),
		tc.organization,
	)
	listRec := httptest.NewRecorder()
	err := tc.handler.ListInventory(listRec, listReq)
	requireAPIErrorStatus(t, err, http.StatusBadRequest)
}

func TestListProductInventoryHandler_Integration(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)
	createdWarehouse := createWarehouseFixtureForHandler(t, tc)
	upsertInventoryForHandler(t, tc, createdWarehouse.ID, tc.productVariant.ID, 25)

	productInventoryReq := requestWithOrganization(
		httptest.NewRequest(
			http.MethodGet,
			"/v1/merchant/products/"+tc.productVariant.ProductID.String()+"/inventory",
			nil,
		),
		tc.organization,
	)
	productRouteCtx := chi.NewRouteContext()
	productRouteCtx.URLParams.Add("id", tc.productVariant.ProductID.String())
	productInventoryReq = productInventoryReq.WithContext(
		context.WithValue(productInventoryReq.Context(), chi.RouteCtxKey, productRouteCtx),
	)
	productInventoryRec := httptest.NewRecorder()
	err := tc.handler.ListProductInventory(productInventoryRec, productInventoryReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, productInventoryRec.Code)

	var productInventory ListInventoryResponse
	require.NoError(t, json.Unmarshal(productInventoryRec.Body.Bytes(), &productInventory))
	require.Len(t, productInventory.Data, 1)
	require.Equal(t, tc.productVariant.ID, productInventory.Data[0].ProductVariantID)
}

func TestListProductInventoryHandler_InvalidProductID(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)

	productInventoryReq := requestWithOrganization(
		httptest.NewRequest(http.MethodGet, "/v1/merchant/products/not-a-uuid/inventory", nil),
		tc.organization,
	)
	productRouteCtx := chi.NewRouteContext()
	productRouteCtx.URLParams.Add("id", "not-a-uuid")
	productInventoryReq = productInventoryReq.WithContext(
		context.WithValue(productInventoryReq.Context(), chi.RouteCtxKey, productRouteCtx),
	)
	productInventoryRec := httptest.NewRecorder()
	err := tc.handler.ListProductInventory(productInventoryRec, productInventoryReq)
	requireAPIErrorStatus(t, err, http.StatusBadRequest)
}
