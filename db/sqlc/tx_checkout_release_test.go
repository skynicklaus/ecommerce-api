//go:build integration

package db_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestReleaseCheckoutReservationsTxCancelsCheckoutAndReleasesInventory(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	variant, warehouse, item := createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 10, true)

	idempotencyKey := "checkout-cancel-tx-" + time.Now().Format("20060102150405.000000000")
	checkout, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, &idempotencyKey),
	)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, checkout.CheckoutSession.ID)

	result, err := testStore.ReleaseCheckoutReservationsTx(t.Context(), db.ReleaseCheckoutReservationsTxParams{
		CheckoutSessionID: checkout.CheckoutSession.ID,
		BuyerOrgID:        buyerOrg.ID,
		BuyerMemberID:     buyer.Member.ID,
		Action:            db.CheckoutReleaseActionCancel,
	})
	require.NoError(t, err)
	require.False(t, result.AlreadyExisted)
	require.Equal(t, string(util.CheckoutSessionStatusCancelled), result.CheckoutSession.Status)
	require.Equal(t, string(util.CheckoutSessionStatusCancelled), result.Payment.Status)
	require.Len(t, result.Orders, 1)
	require.Equal(t, string(util.CheckoutSessionStatusCancelled), result.Orders[0].Status)
	require.Len(t, result.InventoryReservations, 1)
	require.Equal(t, string(util.CheckoutSessionStatusCancelled), result.InventoryReservations[0].Status)
	require.NotNil(t, result.InventoryReservations[0].ReleasedAt)

	inventory, err := testStore.GetWarehouseVariantInventory(t.Context(), db.GetWarehouseVariantInventoryParams{
		OrganizationID:   sellerOrg.ID,
		ProductVariantID: variant.ID,
		WarehouseID:      warehouse.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int32(10), inventory.QuantityOnHand)
	require.Zero(t, inventory.QuantityReserved)

	replayed, err := testStore.ReleaseCheckoutReservationsTx(t.Context(), db.ReleaseCheckoutReservationsTxParams{
		CheckoutSessionID: checkout.CheckoutSession.ID,
		BuyerOrgID:        buyerOrg.ID,
		BuyerMemberID:     buyer.Member.ID,
		Action:            db.CheckoutReleaseActionCancel,
	})
	require.NoError(t, err)
	require.True(t, replayed.AlreadyExisted)
	require.Equal(t, result.CheckoutSession.ID, replayed.CheckoutSession.ID)

	inventory, err = testStore.GetWarehouseVariantInventory(t.Context(), db.GetWarehouseVariantInventoryParams{
		OrganizationID:   sellerOrg.ID,
		ProductVariantID: variant.ID,
		WarehouseID:      warehouse.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int32(10), inventory.QuantityOnHand)
	require.Zero(t, inventory.QuantityReserved)
	require.Equal(t, int32(item.Quantity), result.InventoryReservationItems[0].Quantity)
}

func TestReleaseCheckoutReservationsTxRejectsDifferentBuyerMember(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	otherBuyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 10, true)

	idempotencyKey := "checkout-cancel-member-tx-" + time.Now().Format("20060102150405.000000000")
	checkout, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, &idempotencyKey),
	)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, checkout.CheckoutSession.ID)

	_, err = testStore.ReleaseCheckoutReservationsTx(t.Context(), db.ReleaseCheckoutReservationsTxParams{
		CheckoutSessionID: checkout.CheckoutSession.ID,
		BuyerOrgID:        buyerOrg.ID,
		BuyerMemberID:     otherBuyer.Member.ID,
		Action:            db.CheckoutReleaseActionCancel,
	})
	require.ErrorIs(t, err, db.ErrNotFound)
}

func TestReleaseCheckoutReservationsTxExpiresCheckoutAndReleasesInventory(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	buyer := createCheckoutBuyerContext(t, buyerOrg)
	sellerOrg := createSellerOrganization(t)

	variant, warehouse, _ := createCheckoutCartItem(t, buyerOrg, sellerOrg, 2, 10, true)

	idempotencyKey := "checkout-expire-tx-" + time.Now().Format("20060102150405.000000000")
	checkout, err := testStore.CheckoutSelectedCartItemsTx(
		t.Context(),
		checkoutSelectedCartItemsTxParams(buyer.Customer, buyerOrg, buyer.Member, &idempotencyKey),
	)
	require.NoError(t, err)
	cleanupCheckoutTxResult(t, checkout.CheckoutSession.ID)

	result, err := testStore.ReleaseCheckoutReservationsTx(t.Context(), db.ReleaseCheckoutReservationsTxParams{
		CheckoutSessionID: checkout.CheckoutSession.ID,
		BuyerOrgID:        buyerOrg.ID,
		BuyerMemberID:     buyer.Member.ID,
		Action:            db.CheckoutReleaseActionExpire,
	})
	require.NoError(t, err)
	require.Equal(t, string(util.CheckoutSessionStatusExpired), result.CheckoutSession.Status)
	require.Equal(t, string(util.CheckoutSessionStatusCancelled), result.Payment.Status)
	require.Len(t, result.Orders, 1)
	require.Equal(t, string(util.CheckoutSessionStatusExpired), result.Orders[0].Status)
	require.Len(t, result.InventoryReservations, 1)
	require.Equal(t, string(util.CheckoutSessionStatusExpired), result.InventoryReservations[0].Status)

	inventory, err := testStore.GetWarehouseVariantInventory(t.Context(), db.GetWarehouseVariantInventoryParams{
		OrganizationID:   sellerOrg.ID,
		ProductVariantID: variant.ID,
		WarehouseID:      warehouse.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int32(10), inventory.QuantityOnHand)
	require.Zero(t, inventory.QuantityReserved)
}
