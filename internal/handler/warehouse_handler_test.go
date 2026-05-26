//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestCreateWarehouseHandler_Integration(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)

	rec, err := createWarehouseForHandler(t, tc, "Main Warehouse")
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, rec.Code)

	var createdWarehouse WarehouseResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createdWarehouse))
	require.NotZero(t, createdWarehouse.ID)
	require.Equal(t, tc.organization.ID, createdWarehouse.OrganizationID)
	require.Equal(t, "Main Warehouse", createdWarehouse.Name)
}

func TestCreateWarehouseHandler_ValidationError(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)

	body, err := json.Marshal(CreateWarehouseRequest{
		Address: warehouseAddressRequest("Missing name warehouse"),
	})
	require.NoError(t, err)

	req := requestWithOrganization(
		httptest.NewRequest(http.MethodPost, "/v1/merchant/warehouses", bytes.NewReader(body)),
		tc.organization,
	)
	rec := httptest.NewRecorder()
	err = tc.handler.CreateWarehouse(rec, req)
	requireAPIErrorStatus(t, err, http.StatusUnprocessableEntity)
}

func TestUpdateWarehouseHandler_Integration(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)
	createdWarehouse := createWarehouseFixtureForHandler(t, tc)

	updateReq := UpdateWarehouseRequest{
		Name:     "Updated Warehouse",
		IsActive: new(true),
		Address:  warehouseAddressRequest("Updated warehouse"),
	}
	updateBody, err := json.Marshal(updateReq)
	require.NoError(t, err)

	updateHTTPReq := httptest.NewRequest(
		http.MethodPut,
		"/v1/merchant/warehouses/"+strconv.FormatInt(createdWarehouse.ID, 10),
		bytes.NewReader(updateBody),
	)
	updateHTTPReq = requestWithOrganization(updateHTTPReq, tc.organization)
	updateRouteCtx := chi.NewRouteContext()
	updateRouteCtx.URLParams.Add("id", strconv.FormatInt(createdWarehouse.ID, 10))
	updateHTTPReq = updateHTTPReq.WithContext(
		context.WithValue(updateHTTPReq.Context(), chi.RouteCtxKey, updateRouteCtx),
	)
	updateRec := httptest.NewRecorder()
	err = tc.handler.UpdateWarehouse(updateRec, updateHTTPReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, updateRec.Code)

	var updatedWarehouse WarehouseResponse
	require.NoError(t, json.Unmarshal(updateRec.Body.Bytes(), &updatedWarehouse))
	require.Equal(t, createdWarehouse.ID, updatedWarehouse.ID)
	require.Equal(t, "Updated Warehouse", updatedWarehouse.Name)
}

func TestUpdateWarehouseHandler_NotFound(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)

	req := updateWarehouseRequest(t, tc.organization, 999_999_999, validUpdateWarehouseRequest())
	rec := httptest.NewRecorder()
	err := tc.handler.UpdateWarehouse(rec, req)
	requireAPIErrorStatus(t, err, http.StatusNotFound)
}

func TestUpdateWarehouseHandler_InvalidID(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)

	tests := []struct {
		name string
		id   string
	}{
		{name: "non numeric", id: "abc"},
		{name: "zero", id: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(validUpdateWarehouseRequest())
			require.NoError(t, err)

			req := httptest.NewRequest(
				http.MethodPut,
				"/v1/merchant/warehouses/"+tt.id,
				bytes.NewReader(body),
			)
			req = requestWithOrganization(req, tc.organization)
			routeCtx := chi.NewRouteContext()
			routeCtx.URLParams.Add("id", tt.id)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))

			rec := httptest.NewRecorder()
			err = tc.handler.UpdateWarehouse(rec, req)
			requireAPIErrorStatus(t, err, http.StatusBadRequest)
		})
	}
}

func TestListWarehousesHandler_Integration(t *testing.T) {
	tc := setupWarehouseInventoryHandlerTest(t)
	createWarehouseFixtureForHandler(t, tc)

	listReq := requestWithOrganization(
		httptest.NewRequest(http.MethodGet, "/v1/merchant/warehouses", nil),
		tc.organization,
	)
	listRec := httptest.NewRecorder()
	err := tc.handler.ListWarehouses(listRec, listReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listRec.Code)

	var warehouseList ListWarehousesResponse
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &warehouseList))
	require.Len(t, warehouseList.Data, 1)
}

func createWarehouseForHandler(
	t *testing.T,
	tc warehouseInventoryTestContext,
	name string,
) (*httptest.ResponseRecorder, error) {
	t.Helper()

	createReq := CreateWarehouseRequest{
		Name:    name,
		Address: warehouseAddressRequest("Main warehouse"),
	}
	createBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	req := requestWithOrganization(
		httptest.NewRequest(
			http.MethodPost,
			"/v1/merchant/warehouses",
			bytes.NewReader(createBody),
		),
		tc.organization,
	)
	rec := httptest.NewRecorder()
	return rec, tc.handler.CreateWarehouse(rec, req)
}
