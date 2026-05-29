//go:build integration

package db_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestConfirmManualPaymentTxCompletesCheckout(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	variant, warehouse, item := createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 10, true)

	idempotencyKey := "payment-confirm-tx-" + time.Now().Format("20060102150405.000000000")
	checkout, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, &idempotencyKey),
	)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, checkout.CheckoutSession.ID)

	result, err := testStore.ConfirmManualPaymentTx(t.Context(), db.ConfirmManualPaymentTxParams{
		PaymentID:     checkout.Payment.ID,
		BuyerOrgID:    buyerOrg.ID,
		BuyerMemberID: buyer.Member.ID,
	})
	require.NoError(t, err)
	require.False(t, result.AlreadyExisted)
	require.Equal(t, string(util.CheckoutSessionStatusCompleted), result.CheckoutSession.Status)
	require.Equal(t, string(util.PaymentStatusSucceeded), result.Payment.Status)
	require.Equal(t, checkout.Payment.ID, result.Payment.ID)
	require.Equal(t, string(util.PaymentTransactionTypeSale), result.PaymentTransaction.Type)
	require.Equal(t, string(util.PaymentStatusSucceeded), result.PaymentTransaction.Status)
	require.Equal(t, result.Payment.ID, result.PaymentTransaction.PaymentID)

	require.Len(t, result.Orders, 1)
	for _, order := range result.Orders {
		require.Equal(t, string(util.OrderStatusPlaced), order.Status)
		require.Equal(t, string(util.OrderPaymentStatusPaid), order.PaymentStatus)
		require.NotNil(t, order.PlacedAt)
		require.NotNil(t, order.PaidAt)
	}

	require.Len(t, result.InventoryReservations, 1)
	for _, reservation := range result.InventoryReservations {
		require.Equal(t, string(util.InventoryReservationStatusConfirmed), reservation.Status)
		require.NotNil(t, reservation.ConfirmedAt)
	}
	require.Len(t, result.InventoryReservationItems, 1)

	inventory, err := testStore.GetWarehouseVariantInventory(t.Context(), db.GetWarehouseVariantInventoryParams{
		OrganizationID:   sellerOrg.ID,
		ProductVariantID: variant.ID,
		WarehouseID:      warehouse.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int32(10-item.Quantity), inventory.QuantityOnHand)
	require.Zero(t, inventory.QuantityReserved)

	transactions, err := testStore.ListPaymentTransactionsByPayment(t.Context(), result.Payment.ID)
	require.NoError(t, err)
	require.Len(t, transactions, 1)

	replayed, err := testStore.ConfirmManualPaymentTx(t.Context(), db.ConfirmManualPaymentTxParams{
		PaymentID:     checkout.Payment.ID,
		BuyerOrgID:    buyerOrg.ID,
		BuyerMemberID: buyer.Member.ID,
	})
	require.NoError(t, err)
	require.True(t, replayed.AlreadyExisted)
	require.Equal(t, result.CheckoutSession.ID, replayed.CheckoutSession.ID)
	require.Equal(t, result.PaymentTransaction.ID, replayed.PaymentTransaction.ID)

	transactions, err = testStore.ListPaymentTransactionsByPayment(t.Context(), result.Payment.ID)
	require.NoError(t, err)
	require.Len(t, transactions, 1)
}

func TestConfirmManualPaymentTxRejectsDifferentBuyerMember(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	otherBuyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 10, true)

	idempotencyKey := "payment-confirm-member-tx-" + time.Now().Format("20060102150405.000000000")
	checkout, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, &idempotencyKey),
	)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, checkout.CheckoutSession.ID)

	_, err = testStore.ConfirmManualPaymentTx(t.Context(), db.ConfirmManualPaymentTxParams{
		PaymentID:     checkout.Payment.ID,
		BuyerOrgID:    buyerOrg.ID,
		BuyerMemberID: otherBuyer.Member.ID,
	})
	require.ErrorIs(t, err, db.ErrNotFound)
}

func TestConfirmManualPaymentTxRejectsNonManualProvider(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 10, true)

	idempotencyKey := "payment-confirm-provider-tx-" + time.Now().Format("20060102150405.000000000")
	checkout, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, &idempotencyKey),
	)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, checkout.CheckoutSession.ID)

	_, err = testPool.Exec(t.Context(), "UPDATE payments SET provider = 'stripe' WHERE id = $1", checkout.Payment.ID)
	require.NoError(t, err)

	_, err = testStore.ConfirmManualPaymentTx(t.Context(), db.ConfirmManualPaymentTxParams{
		PaymentID:     checkout.Payment.ID,
		BuyerOrgID:    buyerOrg.ID,
		BuyerMemberID: buyer.Member.ID,
	})
	require.ErrorIs(t, err, db.ErrInvalidPaymentState)
}

func TestConfirmManualPaymentTxRejectsExpiredCheckout(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 10, true)

	idempotencyKey := "payment-confirm-expired-tx-" + time.Now().Format("20060102150405.000000000")
	checkout, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, &idempotencyKey),
	)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, checkout.CheckoutSession.ID)

	_, err = testPool.Exec(
		t.Context(),
		"UPDATE checkout_sessions SET expires_at = NOW() - INTERVAL '1 second' WHERE id = $1",
		checkout.CheckoutSession.ID,
	)
	require.NoError(t, err)

	_, err = testStore.ConfirmManualPaymentTx(t.Context(), db.ConfirmManualPaymentTxParams{
		PaymentID:     checkout.Payment.ID,
		BuyerOrgID:    buyerOrg.ID,
		BuyerMemberID: buyer.Member.ID,
	})
	require.ErrorIs(t, err, db.ErrInvalidCheckoutState)
}
