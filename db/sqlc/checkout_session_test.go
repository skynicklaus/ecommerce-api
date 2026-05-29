//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

type checkoutBuyerContext struct {
	Customer db.Customer
	Member   db.Member
}

func createCheckoutBuyerContext(t *testing.T, buyerOrg db.Organization) checkoutBuyerContext {
	t.Helper()

	identity := createRandomIdentity(t)
	customer, err := testStore.CreateCustomer(t.Context(), db.CreateCustomerParams{
		IdentityID: identity.ID,
		Name:       "buyer " + util.GetRandomString(t, 8),
		Email:      util.GetRandomEmail(t, 10),
	})
	require.NoError(t, err)

	member, err := testStore.CreateMember(t.Context(), db.CreateMemberParams{
		IdentityID:     identity.ID,
		OrganizationID: buyerOrg.ID,
	})
	require.NoError(t, err)

	return checkoutBuyerContext{
		Customer: customer,
		Member:   member,
	}
}

func createCheckoutSessionForBuyer(
	t *testing.T,
	buyerOrg db.Organization,
	buyer checkoutBuyerContext,
	idempotencyKey *string,
) db.CheckoutSession {
	t.Helper()

	zero := decimal.Zero
	checkoutSession, err := testStore.CreateCheckoutSession(t.Context(), db.CreateCheckoutSessionParams{
		BuyerCustomerID:     buyer.Customer.ID,
		BuyerOrgID:          buyerOrg.ID,
		BuyerMemberID:       buyer.Member.ID,
		IdempotencyKey:      idempotencyKey,
		CheckoutFingerprint: "checkout-session-test-" + util.GetRandomString(t, 8),
		Subtotal:            zero,
		TaxTotal:            zero,
		ShippingTotal:       zero,
		DiscountTotal:       zero,
		GrandTotal:          zero,
		Currency:            "MYR",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), "DELETE FROM checkout_sessions WHERE id = $1", checkoutSession.ID)
	})

	return checkoutSession
}

func TestCheckoutSessionIdempotencyScopedToBuyerMember(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	firstBuyer := createCheckoutBuyerContext(t, buyerOrg)
	secondBuyer := createCheckoutBuyerContext(t, buyerOrg)
	idempotencyKey := "checkout-" + util.GetRandomString(t, 12)

	firstSession := createCheckoutSessionForBuyer(t, buyerOrg, firstBuyer, &idempotencyKey)
	secondSession := createCheckoutSessionForBuyer(t, buyerOrg, secondBuyer, &idempotencyKey)
	require.NotEqual(t, firstSession.ID, secondSession.ID)

	firstFetched, err := testStore.GetCheckoutSessionByIdempotencyKey(t.Context(), db.GetCheckoutSessionByIdempotencyKeyParams{
		BuyerOrgID:     buyerOrg.ID,
		BuyerMemberID:  firstBuyer.Member.ID,
		IdempotencyKey: idempotencyKey,
	})
	require.NoError(t, err)
	require.Equal(t, firstSession.ID, firstFetched.ID)

	secondFetched, err := testStore.GetCheckoutSessionByIdempotencyKey(t.Context(), db.GetCheckoutSessionByIdempotencyKeyParams{
		BuyerOrgID:     buyerOrg.ID,
		BuyerMemberID:  secondBuyer.Member.ID,
		IdempotencyKey: idempotencyKey,
	})
	require.NoError(t, err)
	require.Equal(t, secondSession.ID, secondFetched.ID)
}
