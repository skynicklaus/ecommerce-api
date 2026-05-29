//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestCreateCheckoutHandler_Integration(t *testing.T) {
	fixture := newCartHandlerFixture(t)

	t.Run("missing_auth_returns_401", func(t *testing.T) {
		req := newCartJSONRequest(t, http.MethodPost, "/v1/checkout", "", checkoutRequestForTest())
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("merchant_token_is_rejected", func(t *testing.T) {
		req := newCartJSONRequest(t, http.MethodPost, "/v1/checkout", fixture.merchantToken, checkoutRequestForTest())
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("success_and_idempotency_replay", func(t *testing.T) {
		warehouse := createCheckoutInventoryForHandler(t, fixture, fixture.variant.ID, 10)
		cartItemID := addCartItemForTest(t, fixture, fixture.variant.ID, 2)
		idempotencyKey := "checkout-handler-" + uuid.NewString()

		req := newCartJSONRequest(t, http.MethodPost, "/v1/checkout", fixture.customerToken, checkoutRequestForTest())
		req.Header.Set(idempotencyKeyHeader, idempotencyKey)
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusCreated, rr.Code)

		var resp CheckoutResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		cleanupCheckoutHandlerResult(t, fixture.pool, resp.CheckoutSession.ID)
		require.Equal(t, fixture.buyerOrg.ID, resp.CheckoutSession.BuyerOrgID)
		require.Equal(t, string(util.CheckoutSessionStatusReserved), resp.CheckoutSession.Status)
		require.Equal(t, "MYR", resp.CheckoutSession.Currency)
		require.True(t, decimal.NewFromInt(50).Equal(resp.CheckoutSession.GrandTotal))
		require.Len(t, resp.Orders, 1)
		require.Len(t, resp.OrderItems, 1)
		require.Len(t, resp.InventoryReservations, 1)
		require.Len(t, resp.InventoryReservationItems, 1)
		require.Equal(t, string(util.PaymentStatusPending), resp.Payment.Status)
		require.True(t, resp.Payment.Amount.Equal(resp.CheckoutSession.GrandTotal))
		require.NotNil(t, resp.OrderItems[0].WarehouseID)
		require.Equal(t, warehouse.ID, *resp.OrderItems[0].WarehouseID)

		_, err := fixture.store.GetCartItemForBuyerOrg(t.Context(), db.GetCartItemForBuyerOrgParams{
			BuyerOrgID: fixture.buyerOrg.ID,
			CartItemID: cartItemID,
		})
		require.ErrorIs(t, err, db.ErrNotFound)

		replayReq := newCartJSONRequest(t, http.MethodPost, "/v1/checkout", fixture.customerToken, checkoutRequestForTest())
		replayReq.Header.Set(idempotencyKeyHeader, idempotencyKey)
		replayRR := httptest.NewRecorder()
		fixture.router.ServeHTTP(replayRR, replayReq)
		require.Equal(t, http.StatusOK, replayRR.Code)

		var replayResp CheckoutResponse
		require.NoError(t, json.Unmarshal(replayRR.Body.Bytes(), &replayResp))
		require.Equal(t, resp.CheckoutSession.ID, replayResp.CheckoutSession.ID)
		require.Equal(t, resp.Payment.ID, replayResp.Payment.ID)
	})
}

func TestCreateCheckoutHandler_EmptySelectedCartReturnsValidationError(t *testing.T) {
	fixture := newCartHandlerFixture(t)

	req := newCartJSONRequest(t, http.MethodPost, "/v1/checkout", fixture.customerToken, checkoutRequestForTest())
	rr := httptest.NewRecorder()

	fixture.router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
}

func TestCreateCheckoutHandler_InvalidCheckoutRequestReturnsValidationError(t *testing.T) {
	tests := []struct {
		name string
		req  CreateCheckoutRequest
	}{
		{
			name: "shipping_address_is_not_object",
			req: CreateCheckoutRequest{
				ShippingAddressSnapshot: json.RawMessage(`[]`),
			},
		},
		{
			name: "billing_address_is_not_object",
			req: CreateCheckoutRequest{
				ShippingAddressSnapshot: json.RawMessage(`{"country":"MY"}`),
				BillingAddressSnapshot:  json.RawMessage(`"invalid"`),
			},
		},
		{
			name: "unsupported_payment_provider",
			req: CreateCheckoutRequest{
				ShippingAddressSnapshot: json.RawMessage(`{"country":"MY"}`),
				PaymentProvider:         "stripe",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCartHandlerFixture(t)
			req := newCartJSONRequest(t, http.MethodPost, "/v1/checkout", fixture.customerToken, tt.req)
			rr := httptest.NewRecorder()

			fixture.router.ServeHTTP(rr, req)
			require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
		})
	}
}

func TestCreateCheckoutHandler_InsufficientInventoryReturnsConflict(t *testing.T) {
	fixture := newCartHandlerFixture(t)
	createCheckoutInventoryForHandler(t, fixture, fixture.variant.ID, 1)
	cartItemID := addCartItemForTest(t, fixture, fixture.variant.ID, 2)

	req := newCartJSONRequest(t, http.MethodPost, "/v1/checkout", fixture.customerToken, checkoutRequestForTest())
	rr := httptest.NewRecorder()

	fixture.router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusConflict, rr.Code)

	remainingItem, err := fixture.store.GetCartItemForBuyerOrg(t.Context(), db.GetCartItemForBuyerOrgParams{
		BuyerOrgID: fixture.buyerOrg.ID,
		CartItemID: cartItemID,
	})
	require.NoError(t, err)
	require.Equal(t, cartItemID, remainingItem.ID)
}

func TestCreateCheckoutHandler_ActiveCheckoutWithSameSelectionReturnsExisting(t *testing.T) {
	fixture := newCartHandlerFixture(t)
	createCheckoutInventoryForHandler(t, fixture, fixture.variant.ID, 10)
	addCartItemForTest(t, fixture, fixture.variant.ID, 1)

	req := newCartJSONRequest(t, http.MethodPost, "/v1/checkout", fixture.customerToken, checkoutRequestForTest())
	rr := httptest.NewRecorder()
	fixture.router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	var resp CheckoutResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	cleanupCheckoutHandlerResult(t, fixture.pool, resp.CheckoutSession.ID)

	addCartItemForTest(t, fixture, fixture.variant.ID, 1)
	req = newCartJSONRequest(t, http.MethodPost, "/v1/checkout", fixture.customerToken, checkoutRequestForTest())
	rr = httptest.NewRecorder()

	fixture.router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var secondResp CheckoutResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &secondResp))
	require.Equal(t, resp.CheckoutSession.ID, secondResp.CheckoutSession.ID)
	require.Equal(t, resp.Payment.ID, secondResp.Payment.ID)
}

func TestCreateCheckoutHandler_NonTrackedInventoryDoesNotRequireStock(t *testing.T) {
	fixture := newCartHandlerFixture(t)
	_, err := fixture.pool.Exec(t.Context(), "UPDATE product_variants SET track_inventory = FALSE WHERE id = $1", fixture.variant.ID)
	require.NoError(t, err)
	addCartItemForTest(t, fixture, fixture.variant.ID, 2)

	req := newCartJSONRequest(t, http.MethodPost, "/v1/checkout", fixture.customerToken, checkoutRequestForTest())
	rr := httptest.NewRecorder()

	fixture.router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	var resp CheckoutResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	cleanupCheckoutHandlerResult(t, fixture.pool, resp.CheckoutSession.ID)
	require.Len(t, resp.OrderItems, 1)
	require.Nil(t, resp.OrderItems[0].WarehouseID)
	require.Empty(t, resp.InventoryReservationItems)
	require.True(t, decimal.NewFromInt(50).Equal(resp.CheckoutSession.GrandTotal))
}

func TestCreateCheckoutHandler_InactiveSelectedItemReturnsConflict(t *testing.T) {
	t.Run("inactive_variant", func(t *testing.T) {
		fixture := newCartHandlerFixture(t)
		createCheckoutInventoryForHandler(t, fixture, fixture.variant.ID, 10)
		addCartItemForTest(t, fixture, fixture.variant.ID, 1)
		_, err := fixture.pool.Exec(t.Context(), "UPDATE product_variants SET is_active = FALSE WHERE id = $1", fixture.variant.ID)
		require.NoError(t, err)

		req := newCartJSONRequest(t, http.MethodPost, "/v1/checkout", fixture.customerToken, checkoutRequestForTest())
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusConflict, rr.Code)
	})

	t.Run("inactive_product", func(t *testing.T) {
		fixture := newCartHandlerFixture(t)
		createCheckoutInventoryForHandler(t, fixture, fixture.variant.ID, 10)
		addCartItemForTest(t, fixture, fixture.variant.ID, 1)
		_, err := fixture.pool.Exec(
			t.Context(),
			"UPDATE products SET status = 'draft' WHERE id = $1",
			fixture.variant.ProductID,
		)
		require.NoError(t, err)

		req := newCartJSONRequest(t, http.MethodPost, "/v1/checkout", fixture.customerToken, checkoutRequestForTest())
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusConflict, rr.Code)
	})
}

func TestConfirmManualPaymentHandler_Integration(t *testing.T) {
	fixture := newCartHandlerFixture(t)
	warehouse := createCheckoutInventoryForHandler(t, fixture, fixture.variant.ID, 10)
	addCartItemForTest(t, fixture, fixture.variant.ID, 2)

	checkoutReq := newCartJSONRequest(t, http.MethodPost, "/v1/checkout", fixture.customerToken, checkoutRequestForTest())
	checkoutRR := httptest.NewRecorder()
	fixture.router.ServeHTTP(checkoutRR, checkoutReq)
	require.Equal(t, http.StatusCreated, checkoutRR.Code)

	var checkoutResp CheckoutResponse
	require.NoError(t, json.Unmarshal(checkoutRR.Body.Bytes(), &checkoutResp))
	cleanupCheckoutHandlerResult(t, fixture.pool, checkoutResp.CheckoutSession.ID)

	req := newCartJSONRequest(
		t,
		http.MethodPost,
		"/v1/payments/"+checkoutResp.Payment.ID.String()+"/confirm",
		fixture.customerToken,
		nil,
	)
	rr := httptest.NewRecorder()
	fixture.router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp ConfirmPaymentResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Equal(t, checkoutResp.CheckoutSession.ID, resp.CheckoutSession.ID)
	require.Equal(t, string(util.CheckoutSessionStatusCompleted), resp.CheckoutSession.Status)
	require.Equal(t, string(util.PaymentStatusSucceeded), resp.Payment.Status)
	require.Equal(t, string(util.PaymentTransactionTypeSale), resp.PaymentTransaction.Type)
	require.Equal(t, string(util.PaymentStatusSucceeded), resp.PaymentTransaction.Status)
	require.Len(t, resp.Orders, 1)
	require.Equal(t, string(util.OrderStatusPlaced), resp.Orders[0].Status)
	require.Len(t, resp.InventoryReservations, 1)
	require.Equal(t, string(util.InventoryReservationStatusConfirmed), resp.InventoryReservations[0].Status)

	inventory, err := fixture.store.GetWarehouseVariantInventory(t.Context(), db.GetWarehouseVariantInventoryParams{
		OrganizationID:   fixture.sellerOrg.ID,
		ProductVariantID: fixture.variant.ID,
		WarehouseID:      warehouse.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int32(8), inventory.QuantityOnHand)
	require.Zero(t, inventory.QuantityReserved)

	replayReq := newCartJSONRequest(
		t,
		http.MethodPost,
		"/v1/payments/"+checkoutResp.Payment.ID.String()+"/confirm",
		fixture.customerToken,
		nil,
	)
	replayRR := httptest.NewRecorder()
	fixture.router.ServeHTTP(replayRR, replayReq)
	require.Equal(t, http.StatusOK, replayRR.Code)

	var replayResp ConfirmPaymentResponse
	require.NoError(t, json.Unmarshal(replayRR.Body.Bytes(), &replayResp))
	require.Equal(t, resp.CheckoutSession.ID, replayResp.CheckoutSession.ID)
	require.Equal(t, resp.PaymentTransaction.ID, replayResp.PaymentTransaction.ID)
}

func TestCancelCheckoutHandler_Integration(t *testing.T) {
	fixture := newCartHandlerFixture(t)
	warehouse := createCheckoutInventoryForHandler(t, fixture, fixture.variant.ID, 10)
	addCartItemForTest(t, fixture, fixture.variant.ID, 2)

	checkoutReq := newCartJSONRequest(t, http.MethodPost, "/v1/checkout", fixture.customerToken, checkoutRequestForTest())
	checkoutRR := httptest.NewRecorder()
	fixture.router.ServeHTTP(checkoutRR, checkoutReq)
	require.Equal(t, http.StatusCreated, checkoutRR.Code)

	var checkoutResp CheckoutResponse
	require.NoError(t, json.Unmarshal(checkoutRR.Body.Bytes(), &checkoutResp))
	cleanupCheckoutHandlerResult(t, fixture.pool, checkoutResp.CheckoutSession.ID)

	req := newCartJSONRequest(
		t,
		http.MethodPost,
		"/v1/checkout/"+checkoutResp.CheckoutSession.ID.String()+"/cancel",
		fixture.customerToken,
		nil,
	)
	rr := httptest.NewRecorder()
	fixture.router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp CheckoutResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Equal(t, checkoutResp.CheckoutSession.ID, resp.CheckoutSession.ID)
	require.Equal(t, string(util.CheckoutSessionStatusCancelled), resp.CheckoutSession.Status)
	require.Equal(t, string(util.CheckoutSessionStatusCancelled), resp.Payment.Status)
	require.Len(t, resp.Orders, 1)
	require.Equal(t, string(util.CheckoutSessionStatusCancelled), resp.Orders[0].Status)
	require.Len(t, resp.InventoryReservations, 1)
	require.Equal(t, string(util.CheckoutSessionStatusCancelled), resp.InventoryReservations[0].Status)

	inventory, err := fixture.store.GetWarehouseVariantInventory(t.Context(), db.GetWarehouseVariantInventoryParams{
		OrganizationID:   fixture.sellerOrg.ID,
		ProductVariantID: fixture.variant.ID,
		WarehouseID:      warehouse.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int32(10), inventory.QuantityOnHand)
	require.Zero(t, inventory.QuantityReserved)

	replayReq := newCartJSONRequest(
		t,
		http.MethodPost,
		"/v1/checkout/"+checkoutResp.CheckoutSession.ID.String()+"/cancel",
		fixture.customerToken,
		nil,
	)
	replayRR := httptest.NewRecorder()
	fixture.router.ServeHTTP(replayRR, replayReq)
	require.Equal(t, http.StatusOK, replayRR.Code)
}

func checkoutRequestForTest() CreateCheckoutRequest {
	return CreateCheckoutRequest{
		ShippingAddressSnapshot: json.RawMessage(`{"line1":"1 Checkout Road","country":"MY"}`),
		Currency:                "MYR",
		PaymentProvider:         string(util.PaymentProviderManual),
	}
}

func createCheckoutInventoryForHandler(
	t *testing.T,
	fixture cartHandlerFixture,
	variantID uuid.UUID,
	quantityOnHand int32,
) db.Warehouse {
	t.Helper()

	result, err := fixture.store.CreateWarehouseTx(t.Context(), db.CreateWarehouseTxParams{
		OrganizationID: fixture.sellerOrg.ID,
		Name:           "Checkout Warehouse " + uuid.NewString(),
		Address: db.CreateAddressParams{
			Type:       string(util.AddressWarehouse),
			Label:      "Checkout Warehouse",
			Line1:      "1 Checkout Warehouse Road",
			PostalCode: "50000",
			City:       "Kuala Lumpur",
			State:      "Kuala Lumpur",
			Country:    "MY",
		},
	})
	require.NoError(t, err)

	_, err = fixture.pool.Exec(t.Context(), "UPDATE warehouses SET is_active = TRUE WHERE id = $1", result.Warehouse.ID)
	require.NoError(t, err)
	result.Warehouse.IsActive = true

	_, err = fixture.store.CreateInventory(t.Context(), db.CreateInventoryParams{
		ProductVariantID: variantID,
		WarehouseID:      result.Warehouse.ID,
		QuantityOnHand:   quantityOnHand,
	})
	require.NoError(t, err)

	return result.Warehouse
}

func cleanupCheckoutHandlerResult(t *testing.T, pool *pgxpool.Pool, checkoutSessionID uuid.UUID) {
	t.Helper()

	t.Cleanup(func() {
		ctx := context.Background()

		_, _ = pool.Exec(ctx, `
			UPDATE inventories inv
			SET quantity_reserved = GREATEST(0, inv.quantity_reserved - reserved.quantity)
			FROM (
				SELECT iri.product_variant_id, iri.warehouse_id, SUM(iri.quantity)::INTEGER AS quantity
				FROM inventory_reservation_items iri
				JOIN inventory_reservations ir ON ir.id = iri.reservation_id
				WHERE ir.checkout_session_id = $1
				GROUP BY iri.product_variant_id, iri.warehouse_id
			) reserved
			WHERE inv.product_variant_id = reserved.product_variant_id
				AND inv.warehouse_id = reserved.warehouse_id
		`, checkoutSessionID)
		_, _ = pool.Exec(ctx, `
			DELETE FROM payment_transactions
			WHERE payment_id IN (
				SELECT id FROM payments WHERE checkout_session_id = $1
			)
		`, checkoutSessionID)
		_, _ = pool.Exec(ctx, "DELETE FROM payments WHERE checkout_session_id = $1", checkoutSessionID)
		_, _ = pool.Exec(ctx, `
			DELETE FROM inventory_reservation_items
			WHERE reservation_id IN (
				SELECT id FROM inventory_reservations WHERE checkout_session_id = $1
			)
		`, checkoutSessionID)
		_, _ = pool.Exec(ctx, "DELETE FROM inventory_reservations WHERE checkout_session_id = $1", checkoutSessionID)
		_, _ = pool.Exec(ctx, `
			DELETE FROM order_status_histories
			WHERE order_id IN (
				SELECT id FROM orders WHERE checkout_session_id = $1
			)
		`, checkoutSessionID)
		_, _ = pool.Exec(ctx, `
			DELETE FROM order_items
			WHERE order_id IN (
				SELECT id FROM orders WHERE checkout_session_id = $1
			)
		`, checkoutSessionID)
		_, _ = pool.Exec(ctx, "DELETE FROM orders WHERE checkout_session_id = $1", checkoutSessionID)
		_, _ = pool.Exec(ctx, "DELETE FROM checkout_sessions WHERE id = $1", checkoutSessionID)
	})
}
