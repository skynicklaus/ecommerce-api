//go:build integration

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createCheckoutCartItem(
	t *testing.T,
	buyerOrg db.Organization,
	sellerOrg db.Organization,
	quantity int16,
	quantityOnHand int32,
	selected bool,
) (db.ProductVariant, db.Warehouse, db.CartItem) {
	t.Helper()

	variant := createActiveVariantForCart(t, sellerOrg)
	warehouse := createRandomWarehouseWithOrg(t, sellerOrg)
	_, err := testPool.Exec(t.Context(), "UPDATE warehouses SET is_active = TRUE WHERE id = $1", warehouse.ID)
	require.NoError(t, err)
	warehouse.IsActive = true

	_, err = testStore.CreateInventory(t.Context(), db.CreateInventoryParams{
		ProductVariantID: variant.ID,
		WarehouseID:      warehouse.ID,
		QuantityOnHand:   quantityOnHand,
	})
	require.NoError(t, err)

	added, err := testStore.AddCartItemTx(t.Context(), db.AddCartItemTxParams{
		BuyerOrgID:       buyerOrg.ID,
		ProductVariantID: variant.ID,
		Quantity:         quantity,
	})
	require.NoError(t, err)

	selectedItem, err := testStore.SetCartItemSelectedTx(t.Context(), db.SetCartItemSelectedTxParams{
		BuyerOrgID: buyerOrg.ID,
		CartItemID: added.Item.ID,
		IsSelected: selected,
	})
	require.NoError(t, err)
	added.Item = selectedItem.Item

	return variant, warehouse, added.Item
}

func cleanupCheckoutTxResult(t *testing.T, checkoutSessionID uuid.UUID) {
	t.Helper()

	t.Cleanup(func() {
		ctx := context.Background()

		_, _ = testPool.Exec(ctx, `
			DELETE FROM payment_transactions
			WHERE payment_id IN (
				SELECT id FROM payments WHERE checkout_session_id = $1
			)
		`, checkoutSessionID)
		_, _ = testPool.Exec(ctx, "DELETE FROM payments WHERE checkout_session_id = $1", checkoutSessionID)
		_, _ = testPool.Exec(ctx, `
			DELETE FROM inventory_reservation_items
			WHERE reservation_id IN (
				SELECT id FROM inventory_reservations WHERE checkout_session_id = $1
			)
		`, checkoutSessionID)
		_, _ = testPool.Exec(ctx, "DELETE FROM inventory_reservations WHERE checkout_session_id = $1", checkoutSessionID)
		_, _ = testPool.Exec(ctx, `
			DELETE FROM order_status_histories
			WHERE order_id IN (
				SELECT id FROM orders WHERE checkout_session_id = $1
			)
		`, checkoutSessionID)
		_, _ = testPool.Exec(ctx, `
			DELETE FROM order_items
			WHERE order_id IN (
				SELECT id FROM orders WHERE checkout_session_id = $1
			)
		`, checkoutSessionID)
		_, _ = testPool.Exec(ctx, "DELETE FROM orders WHERE checkout_session_id = $1", checkoutSessionID)
		_, _ = testPool.Exec(ctx, "DELETE FROM checkout_sessions WHERE id = $1", checkoutSessionID)
	})
}

func checkoutSelectedCartItemsTxParams(
	buyer db.Customer,
	buyerOrg db.Organization,
	buyerMember db.Member,
	idempotencyKey *string,
) db.CheckoutSelectedCartItemsTxParams {
	return db.CheckoutSelectedCartItemsTxParams{
		BuyerCustomerID:         buyer.ID,
		BuyerOrgID:              buyerOrg.ID,
		BuyerMemberID:           buyerMember.ID,
		IdempotencyKey:          idempotencyKey,
		CustomerEmail:           buyer.Email,
		CustomerName:            buyer.Name,
		ShippingAddressSnapshot: []byte(`{"country":"MY"}`),
		Currency:                "MYR",
		ReservationTTL:          30 * time.Minute,
		PaymentProvider:         string(util.PaymentProviderManual),
	}
}

func requireNoActiveCheckoutSession(t *testing.T, buyerOrg db.Organization, buyerMember db.Member) {
	t.Helper()

	_, err := testStore.GetActiveCheckoutSessionForBuyer(t.Context(), db.GetActiveCheckoutSessionForBuyerParams{
		BuyerOrgID:    buyerOrg.ID,
		BuyerMemberID: buyerMember.ID,
	})
	require.ErrorIs(t, err, db.ErrNotFound)
}

func TestCheckoutSelectedCartItemsTxCreatesMultiMerchantCheckout(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	firstSellerOrg := createSellerOrganization(t)
	secondSellerOrg := createSellerOrganization(t)

	firstVariant, firstWarehouse, firstItem := createCheckoutCartItem(t, buyerOrg, firstSellerOrg, 2, 10, true)
	secondVariant, secondWarehouse, secondItem := createCheckoutCartItem(t, buyerOrg, secondSellerOrg, 3, 10, true)
	_, _, unselectedItem := createCheckoutCartItem(t, buyerOrg, firstSellerOrg, 1, 10, false)

	idempotencyKey := "checkout-tx-" + time.Now().Format("20060102150405.000000000")
	result, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, &idempotencyKey),
	)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, result.CheckoutSession.ID)
	require.Equal(t, buyerOrg.ID, result.CheckoutSession.BuyerOrgID)
	require.Equal(t, buyer.Member.ID, result.CheckoutSession.BuyerMemberID)
	require.Equal(t, string(util.CheckoutSessionStatusReserved), result.CheckoutSession.Status)
	require.Equal(t, "MYR", result.CheckoutSession.Currency)
	require.True(t, result.CheckoutSession.Subtotal.Equal(result.CheckoutSession.GrandTotal))
	require.True(t, result.CheckoutSession.GrandTotal.GreaterThan(decimal.Zero))

	require.Len(t, result.Orders, 2)
	require.Len(t, result.OrderItems, 2)
	require.Len(t, result.InventoryReservations, 2)
	require.Len(t, result.InventoryReservationItems, 2)
	require.NotZero(t, result.Payment.ID)
	require.True(t, result.Payment.Amount.Equal(result.CheckoutSession.GrandTotal))

	orderByMerchant := map[string]db.Order{}
	for _, order := range result.Orders {
		require.Equal(t, result.CheckoutSession.ID, order.CheckoutSessionID)
		require.Equal(t, buyerOrg.ID, order.BuyerOrgID)
		require.Equal(t, buyer.Member.ID, order.BuyerMemberID)
		require.Equal(t, string(util.OrderStatusPendingPayment), order.Status)
		orderByMerchant[order.MerchantOrgID.String()] = order
	}
	require.Contains(t, orderByMerchant, firstSellerOrg.ID.String())
	require.Contains(t, orderByMerchant, secondSellerOrg.ID.String())

	reservedByVariant := map[string]int32{}
	for _, reservationItem := range result.InventoryReservationItems {
		reservedByVariant[reservationItem.ProductVariantID.String()] += reservationItem.Quantity
	}
	require.Equal(t, int32(firstItem.Quantity), reservedByVariant[firstVariant.ID.String()])
	require.Equal(t, int32(secondItem.Quantity), reservedByVariant[secondVariant.ID.String()])

	firstInventory, err := testStore.GetWarehouseVariantInventory(t.Context(), db.GetWarehouseVariantInventoryParams{
		OrganizationID:   firstSellerOrg.ID,
		ProductVariantID: firstVariant.ID,
		WarehouseID:      firstWarehouse.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int32(firstItem.Quantity), firstInventory.QuantityReserved)

	secondInventory, err := testStore.GetWarehouseVariantInventory(t.Context(), db.GetWarehouseVariantInventoryParams{
		OrganizationID:   secondSellerOrg.ID,
		ProductVariantID: secondVariant.ID,
		WarehouseID:      secondWarehouse.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int32(secondItem.Quantity), secondInventory.QuantityReserved)

	_, err = testStore.GetCartItemForBuyerOrg(t.Context(), db.GetCartItemForBuyerOrgParams{
		BuyerOrgID: buyerOrg.ID,
		CartItemID: firstItem.ID,
	})
	require.ErrorIs(t, err, db.ErrNotFound)

	_, err = testStore.GetCartItemForBuyerOrg(t.Context(), db.GetCartItemForBuyerOrgParams{
		BuyerOrgID: buyerOrg.ID,
		CartItemID: secondItem.ID,
	})
	require.ErrorIs(t, err, db.ErrNotFound)

	remainingItem, err := testStore.GetCartItemForBuyerOrg(t.Context(), db.GetCartItemForBuyerOrgParams{
		BuyerOrgID: buyerOrg.ID,
		CartItemID: unselectedItem.ID,
	})
	require.NoError(t, err)
	require.Equal(t, unselectedItem.ID, remainingItem.ID)
}

func TestCheckoutSelectedCartItemsTxReturnsExistingCheckoutForIdempotencyReplay(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 10, true)

	idempotencyKey := "checkout-tx-replay-" + time.Now().Format("20060102150405.000000000")
	arg := checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, &idempotencyKey)

	firstResult, err := testStore.CheckoutSelectedCartItemsTx(t.Context(), arg)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, firstResult.CheckoutSession.ID)

	secondResult, err := testStore.CheckoutSelectedCartItemsTx(t.Context(), arg)
	require.NoError(t, err)
	require.Equal(t, firstResult.CheckoutSession.ID, secondResult.CheckoutSession.ID)
	require.Equal(t, firstResult.Payment.ID, secondResult.Payment.ID)
	require.Len(t, secondResult.Orders, len(firstResult.Orders))
	require.Len(t, secondResult.OrderItems, len(firstResult.OrderItems))
	require.Len(t, secondResult.InventoryReservations, len(firstResult.InventoryReservations))
	require.Len(t, secondResult.InventoryReservationItems, len(firstResult.InventoryReservationItems))
}

func TestCheckoutSelectedCartItemsTxReturnsActiveCheckoutWhenCartSelectionIsEmpty(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 10, true)

	firstResult, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, nil),
	)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, firstResult.CheckoutSession.ID)

	secondResult, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, nil),
	)
	require.NoError(t, err)
	require.True(t, secondResult.AlreadyExisted)
	require.Equal(t, firstResult.CheckoutSession.ID, secondResult.CheckoutSession.ID)
	require.Equal(t, firstResult.Payment.ID, secondResult.Payment.ID)
}

func TestCheckoutSelectedCartItemsTxNormalizesAddressSnapshotFingerprint(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	variant, _, _ := createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 10, true)

	firstArg := checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, nil)
	firstArg.ShippingAddressSnapshot = []byte(`{"country":"MY","city":"KL"}`)
	firstResult, err := testStore.CheckoutSelectedCartItemsTx(t.Context(), firstArg)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, firstResult.CheckoutSession.ID)

	added, err := testStore.AddCartItemTx(t.Context(), db.AddCartItemTxParams{
		BuyerOrgID:       buyerOrg.ID,
		ProductVariantID: variant.ID,
		Quantity:         2,
	})
	require.NoError(t, err)
	_, err = testStore.SetCartItemSelectedTx(t.Context(), db.SetCartItemSelectedTxParams{
		BuyerOrgID: buyerOrg.ID,
		CartItemID: added.Item.ID,
		IsSelected: true,
	})
	require.NoError(t, err)

	secondArg := checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, nil)
	secondArg.ShippingAddressSnapshot = []byte(`{"city":"KL","country":"MY"}`)
	secondResult, err := testStore.CheckoutSelectedCartItemsTx(t.Context(), secondArg)
	require.NoError(t, err)
	require.True(t, secondResult.AlreadyExisted)
	require.Equal(t, firstResult.CheckoutSession.ID, secondResult.CheckoutSession.ID)
	require.Equal(t, firstResult.CheckoutSession.CheckoutFingerprint, secondResult.CheckoutSession.CheckoutFingerprint)
}

func TestCheckoutSelectedCartItemsTxReplacesActiveCheckoutWhenSelectionDiffers(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	firstSellerOrg := createSellerOrganization(t)
	secondSellerOrg := createSellerOrganization(t)

	firstVariant, firstWarehouse, firstItem := createCheckoutCartItem(t, buyerOrg, firstSellerOrg, 2, 10, true)
	firstResult, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, nil),
	)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, firstResult.CheckoutSession.ID)

	secondVariant, secondWarehouse, secondItem := createCheckoutCartItem(t, buyerOrg, secondSellerOrg, 1, 10, true)
	secondResult, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, nil),
	)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, secondResult.CheckoutSession.ID)
	require.False(t, secondResult.AlreadyExisted)
	require.NotEqual(t, firstResult.CheckoutSession.ID, secondResult.CheckoutSession.ID)

	releasedCheckout, err := testStore.GetCheckoutSessionByID(t.Context(), firstResult.CheckoutSession.ID)
	require.NoError(t, err)
	require.Equal(t, string(util.CheckoutSessionStatusCancelled), releasedCheckout.Status)

	firstInventory, err := testStore.GetWarehouseVariantInventory(t.Context(), db.GetWarehouseVariantInventoryParams{
		OrganizationID:   firstSellerOrg.ID,
		ProductVariantID: firstVariant.ID,
		WarehouseID:      firstWarehouse.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int32(10), firstInventory.QuantityOnHand)
	require.Zero(t, firstInventory.QuantityReserved)
	require.Equal(t, int32(firstItem.Quantity), firstResult.InventoryReservationItems[0].Quantity)

	secondInventory, err := testStore.GetWarehouseVariantInventory(t.Context(), db.GetWarehouseVariantInventoryParams{
		OrganizationID:   secondSellerOrg.ID,
		ProductVariantID: secondVariant.ID,
		WarehouseID:      secondWarehouse.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int32(secondItem.Quantity), secondInventory.QuantityReserved)
}

func TestCheckoutSelectedCartItemsTxRejectsInactiveVariant(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	variant, _, item := createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 10, true)
	_, err := testPool.Exec(t.Context(), "UPDATE product_variants SET is_active = FALSE WHERE id = $1", variant.ID)
	require.NoError(t, err)

	idempotencyKey := "checkout-tx-inactive-variant-" + time.Now().Format("20060102150405.000000000")
	_, err = testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, &idempotencyKey),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "inactive")

	remainingItem, err := testStore.GetCartItemForBuyerOrg(t.Context(), db.GetCartItemForBuyerOrgParams{
		BuyerOrgID: buyerOrg.ID,
		CartItemID: item.ID,
	})
	require.NoError(t, err)
	require.Equal(t, item.ID, remainingItem.ID)
	requireNoActiveCheckoutSession(t, buyerOrg, buyer.Member)
}

func TestCheckoutSelectedCartItemsTxRejectsInactiveProduct(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	variant, _, item := createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 10, true)
	_, err := testPool.Exec(t.Context(), "UPDATE products SET status = 'draft' WHERE id = $1", variant.ProductID)
	require.NoError(t, err)

	idempotencyKey := "checkout-tx-inactive-product-" + time.Now().Format("20060102150405.000000000")
	_, err = testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, &idempotencyKey),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not active")

	remainingItem, err := testStore.GetCartItemForBuyerOrg(t.Context(), db.GetCartItemForBuyerOrgParams{
		BuyerOrgID: buyerOrg.ID,
		CartItemID: item.ID,
	})
	require.NoError(t, err)
	require.Equal(t, item.ID, remainingItem.ID)
	requireNoActiveCheckoutSession(t, buyerOrg, buyer.Member)
}

func TestCheckoutSelectedCartItemsTxRollsBackOnInsufficientInventory(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	variant, warehouse, item := createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 1, true)

	idempotencyKey := "checkout-tx-insufficient-" + time.Now().Format("20060102150405.000000000")
	_, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, &idempotencyKey),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient inventory")

	remainingItem, err := testStore.GetCartItemForBuyerOrg(t.Context(), db.GetCartItemForBuyerOrgParams{
		BuyerOrgID: buyerOrg.ID,
		CartItemID: item.ID,
	})
	require.NoError(t, err)
	require.Equal(t, item.ID, remainingItem.ID)
	requireNoActiveCheckoutSession(t, buyerOrg, buyer.Member)

	inventory, err := testStore.GetWarehouseVariantInventory(t.Context(), db.GetWarehouseVariantInventoryParams{
		OrganizationID:   sellerOrg.ID,
		ProductVariantID: variant.ID,
		WarehouseID:      warehouse.ID,
	})
	require.NoError(t, err)
	require.Zero(t, inventory.QuantityReserved)
}

func TestCheckoutSelectedCartItemsTxAllowsNonTrackedInventoryWithoutStockRows(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	variant, _, _ := createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 0, true)
	_, err := testPool.Exec(t.Context(), "DELETE FROM inventories WHERE product_variant_id = $1", variant.ID)
	require.NoError(t, err)
	_, err = testPool.Exec(t.Context(), "UPDATE product_variants SET track_inventory = FALSE WHERE id = $1", variant.ID)
	require.NoError(t, err)

	idempotencyKey := "checkout-tx-non-tracked-" + time.Now().Format("20060102150405.000000000")
	result, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, &idempotencyKey),
	)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, result.CheckoutSession.ID)

	require.Len(t, result.Orders, 1)
	require.Len(t, result.OrderItems, 1)
	require.False(t, result.OrderItems[0].WarehouseID.Valid)
	require.Len(t, result.InventoryReservations, 1)
	require.Empty(t, result.InventoryReservationItems)
	require.True(t, result.CheckoutSession.GrandTotal.GreaterThan(decimal.Zero))
}
