//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
	"github.com/skynicklaus/ecommerce-api/internal/middleware"
	"github.com/skynicklaus/ecommerce-api/internal/storage"
	"github.com/skynicklaus/ecommerce-api/util"
)

type warehouseInventoryTestContext struct {
	handler        *V1Handler
	store          db.Store
	connPool       *pgxpool.Pool
	organization   db.Organization
	productVariant db.ProductVariant
}

func setupWarehouseInventoryHandlerTest(t *testing.T) warehouseInventoryTestContext {
	t.Helper()

	ctx := t.Context()
	dbSource := os.Getenv("DB_SOURCE")
	if dbSource == "" {
		t.Skip("DB_SOURCE not set")
	}

	connPool, err := pgxpool.New(ctx, dbSource)
	require.NoError(t, err)
	t.Cleanup(connPool.Close)

	store := db.NewStore(connPool)
	logger := util.NewLogger()
	redisClient := cache.New(store, logger)
	t.Cleanup(func() {
		require.NoError(t, redisClient.Close())
	})

	s3Storage, err := storage.New(ctx)
	require.NoError(t, err)

	h := NewV1Handler(store, logger, redisClient, s3Storage)

	org, productVariant := createWarehouseInventoryFixture(t, ctx, store)
	t.Cleanup(func() {
		_, _ = connPool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
	})

	return warehouseInventoryTestContext{
		handler:        h,
		store:          store,
		connPool:       connPool,
		organization:   org,
		productVariant: productVariant,
	}
}

func createWarehouseFixtureForHandler(
	t *testing.T,
	tc warehouseInventoryTestContext,
) db.Warehouse {
	t.Helper()

	result, err := tc.store.CreateWarehouseTx(t.Context(), db.CreateWarehouseTxParams{
		OrganizationID: tc.organization.ID,
		Name:           "Inventory Warehouse",
		Address: db.CreateAddressParams{
			Type:       string(util.AddressWarehouse),
			Label:      "Inventory warehouse",
			Line1:      "789 Storage Road",
			PostalCode: "11111",
			City:       "Austin",
			State:      "TX",
			Country:    "US",
		},
	})
	require.NoError(t, err)
	return result.Warehouse
}

func upsertInventoryForHandler(
	t *testing.T,
	tc warehouseInventoryTestContext,
	warehouseID int64,
	productVariantID uuid.UUID,
	quantity int32,
) {
	t.Helper()

	inventoryReq := UpsertInventoryRequest{
		ProductVariantID: productVariantID,
		WarehouseID:      warehouseID,
		QuantityOnHand:   quantity,
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
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, inventoryRec.Code)
}

func requestWithOrganization(req *http.Request, organization db.Organization) *http.Request {
	return req.WithContext(
		context.WithValue(req.Context(), middleware.OrganizationContextKey{}, organization),
	)
}

func updateWarehouseRequest(
	t *testing.T,
	organization db.Organization,
	warehouseID int64,
	updateReq UpdateWarehouseRequest,
) *http.Request {
	t.Helper()

	body, err := json.Marshal(updateReq)
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPut,
		"/v1/merchant/warehouses/"+strconv.FormatInt(warehouseID, 10),
		bytes.NewReader(body),
	)
	req = requestWithOrganization(req, organization)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", strconv.FormatInt(warehouseID, 10))
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func validUpdateWarehouseRequest() UpdateWarehouseRequest {
	return UpdateWarehouseRequest{
		Name:     "Updated Warehouse",
		IsActive: new(true),
		Address:  warehouseAddressRequest("Updated warehouse"),
	}
}

func warehouseAddressRequest(label string) WarehouseAddressRequest {
	return WarehouseAddressRequest{
		Label:      label,
		Line1:      "123 Storage Road",
		PostalCode: "12345",
		City:       "Austin",
		State:      "TX",
		Country:    "US",
	}
}

func requireAPIErrorStatus(t *testing.T, err error, status int) {
	t.Helper()

	var apiErr apierror.APIError
	require.True(t, errors.As(err, &apiErr), "expected APIError, got: %v", err)
	require.Equal(t, status, apiErr.StatusCode)
}

func createWarehouseInventoryFixture(
	t *testing.T,
	ctx context.Context,
	store db.Store,
) (db.Organization, db.ProductVariant) {
	t.Helper()

	org, err := store.CreateOrganization(ctx, db.CreateOrganizationParams{
		Name:     "merchant-" + uuid.NewString(),
		Slug:     "merchant-" + uuid.NewString(),
		Status:   string(util.OrganizationStatusActive),
		Type:     string(util.OrganizationTypeMerchant),
		Metadata: []byte("{}"),
	})
	require.NoError(t, err)

	orgID := org.ID
	category, err := store.CreateCategory(ctx, db.CreateCategoryParams{
		OrganizationID: &orgID,
		Name:           "category-" + uuid.NewString(),
		Slug:           "category-" + uuid.NewString(),
	})
	require.NoError(t, err)

	product, err := store.CreateProduct(ctx, db.CreateProductParams{
		OrganizationID: org.ID,
		CategoryID:     category.ID,
		Name:           "product-" + uuid.NewString(),
		Slug:           "product-" + uuid.NewString(),
		Description:    []byte(`{"text":"inventory test"}`),
		Specification:  []byte(`{"weight":"1kg"}`),
	})
	require.NoError(t, err)

	variant, err := store.CreateProductVariant(ctx, db.CreateProductVariantParams{
		ProductID:      product.ID,
		OrganizationID: org.ID,
		Sku:            "sku-" + uuid.NewString(),
		Name:           "Variant",
		Price:          decimal.NewFromInt(10),
	})
	require.NoError(t, err)

	return org, variant
}
