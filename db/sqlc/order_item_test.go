//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createCheckoutOrderForMerchant(t *testing.T, buyerOrg, sellerOrg db.Organization) db.Order {
	t.Helper()

	buyer := createCheckoutBuyerContext(t, buyerOrg)
	checkoutSession := createCheckoutSessionForBuyer(t, buyerOrg, buyer, nil)
	zero := decimal.Zero

	order, err := testStore.CreateOrder(t.Context(), db.CreateOrderParams{
		CheckoutSessionID:       checkoutSession.ID,
		MerchantOrgID:           sellerOrg.ID,
		BuyerCustomerID:         buyer.Customer.ID,
		BuyerOrgID:              buyerOrg.ID,
		BuyerMemberID:           buyer.Member.ID,
		OrderNumber:             "ord-" + util.GetRandomString(t, 12),
		CustomerEmail:           buyer.Customer.Email,
		CustomerName:            buyer.Customer.Name,
		ShippingAddressSnapshot: []byte(`{"country":"MY"}`),
		Subtotal:                zero,
		TaxTotal:                zero,
		ShippingTotal:           zero,
		ShippingDiscount:        zero,
		CouponDiscount:          zero,
		GrandTotal:              zero,
		Currency:                "MYR",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), "DELETE FROM orders WHERE id = $1", order.ID)
	})

	return order
}

func createOrderItemParams(order db.Order, variant db.ProductVariant, warehouse db.Warehouse) db.CreateOrderItemParams {
	productID := variant.ProductID
	variantID := variant.ID

	return db.CreateOrderItemParams{
		OrderID:          order.ID,
		ProductID:        &productID,
		ProductVariantID: &variantID,
		WarehouseID: pgtype.Int8{
			Int64: warehouse.ID,
			Valid: true,
		},
		ProductName:   "product snapshot",
		VariantName:   "variant snapshot",
		Sku:           variant.Sku,
		Quantity:      1,
		UnitPrice:     decimal.Zero,
		Subtotal:      decimal.Zero,
		DiscountTotal: decimal.Zero,
		TaxTotal:      decimal.Zero,
		Total:         decimal.Zero,
		Currency:      "MYR",
	}
}

func TestCreateOrderItemValidatesMerchantOwnership(t *testing.T) {
	buyerOrg := createBuyerOrganization(t)
	sellerOrg := createSellerOrganization(t)
	otherSellerOrg := createSellerOrganization(t)
	order := createCheckoutOrderForMerchant(t, buyerOrg, sellerOrg)

	sellerVariant := createRandomProductVariantWithOrg(t, sellerOrg)
	sellerWarehouse := createRandomWarehouseWithOrg(t, sellerOrg)
	otherVariant := createRandomProductVariantWithOrg(t, otherSellerOrg)
	otherWarehouse := createRandomWarehouseWithOrg(t, otherSellerOrg)

	validItem, err := testStore.CreateOrderItem(t.Context(), createOrderItemParams(order, sellerVariant, sellerWarehouse))
	require.NoError(t, err)
	require.Equal(t, order.ID, validItem.OrderID)

	wrongVariantParams := createOrderItemParams(order, otherVariant, sellerWarehouse)
	wrongVariantParams.ProductID = nil
	_, err = testStore.CreateOrderItem(t.Context(), wrongVariantParams)
	require.Error(t, err)

	wrongWarehouseParams := createOrderItemParams(order, sellerVariant, otherWarehouse)
	_, err = testStore.CreateOrderItem(t.Context(), wrongWarehouseParams)
	require.Error(t, err)
}
