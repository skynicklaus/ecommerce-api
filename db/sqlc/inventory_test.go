//go:build integration

package db_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomInventoryWithOrg(t *testing.T, organization db.Organization) db.Inventory {
	t.Helper()
	warehouse := createRandomWarehouseWithOrg(t, organization)
	productVariant := createRandomProductVariantWithOrg(t, organization)

	n := util.CoinFlip(t)
	var lowStockThreashold *int32
	if n == 1 {
		lowStockThreashold = util.GetRandomNumberPtr(t, 5)
	}

	arg := db.CreateInventoryParams{
		ProductVariantID:  productVariant.ID,
		WarehouseID:       warehouse.ID,
		QuantityOnHand:    util.GetRandomNumber(t, 200),
		LowStockThreshold: lowStockThreashold,
	}

	inventory, err := testStore.CreateInventory(t.Context(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, inventory)

	require.Equal(t, arg.ProductVariantID, inventory.ProductVariantID)
	require.Equal(t, arg.WarehouseID, inventory.WarehouseID)
	require.Equal(t, arg.QuantityOnHand, inventory.QuantityOnHand)

	if n != 1 {
		require.Empty(t, inventory.LowStockThreshold)
	} else {
		require.Equal(t, *arg.LowStockThreshold, *inventory.LowStockThreshold)
	}

	return inventory
}

func createRandomInventory(t *testing.T) db.Inventory {
	t.Helper()
	organization := createRandomOrganization(t)
	return createRandomInventoryWithOrg(t, organization)
}

func TestCreateInventory(t *testing.T) {
	createRandomInventory(t)
}

func TestGetWarehouseVariantInventory(t *testing.T) {
	organization := createRandomOrganization(t)
	cleanupOrganization(t, organization.ID.String())
	inventory1 := createRandomInventoryWithOrg(t, organization)

	inventory2, err := testStore.GetWarehouseVariantInventory(
		t.Context(),
		db.GetWarehouseVariantInventoryParams{
			OrganizationID:   organization.ID,
			ProductVariantID: inventory1.ProductVariantID,
			WarehouseID:      inventory1.WarehouseID,
		},
	)
	require.NoError(t, err)
	require.NotEmpty(t, inventory2)

	require.Equal(t, inventory1.ProductVariantID, inventory2.ProductVariantID)
	require.Equal(t, inventory1.WarehouseID, inventory2.WarehouseID)
	require.Equal(t, inventory1.QuantityOnHand, inventory2.QuantityOnHand)
	require.Equal(t, inventory1.QuantityReserved, inventory2.QuantityReserved)
	require.Equal(t, inventory1.QuantityAvailable, inventory2.QuantityAvailable)
	require.Equal(t, inventory1.IsActive, inventory2.IsActive)
}

func TestUpsertInventory(t *testing.T) {
	organization := createRandomOrganization(t)
	cleanupOrganization(t, organization.ID.String())
	warehouse := createRandomWarehouseWithOrg(t, organization)
	productVariant := createRandomProductVariantWithOrg(t, organization)
	threshold := int32(7)

	arg := db.UpsertInventoryParams{
		OrganizationID:    organization.ID,
		ProductVariantID:  productVariant.ID,
		WarehouseID:       warehouse.ID,
		QuantityOnHand:    20,
		LowStockThreshold: &threshold,
		IsActive:          true,
	}
	inventory, err := testStore.UpsertInventory(t.Context(), arg)
	require.NoError(t, err)
	require.Equal(t, arg.ProductVariantID, inventory.ProductVariantID)
	require.Equal(t, arg.WarehouseID, inventory.WarehouseID)
	require.Equal(t, arg.QuantityOnHand, inventory.QuantityOnHand)
	require.Equal(t, threshold, *inventory.LowStockThreshold)
	require.True(t, inventory.IsActive)

	arg.QuantityOnHand = 35
	arg.LowStockThreshold = nil
	arg.IsActive = false
	updated, err := testStore.UpsertInventory(t.Context(), arg)
	require.NoError(t, err)
	require.Equal(t, int32(35), updated.QuantityOnHand)
	require.Nil(t, updated.LowStockThreshold)
	require.False(t, updated.IsActive)
}

func TestUpsertInventoryRejectsCrossOrgReferences(t *testing.T) {
	organization := createRandomOrganization(t)
	cleanupOrganization(t, organization.ID.String())
	otherOrganization := createRandomOrganization(t)
	cleanupOrganization(t, otherOrganization.ID.String())

	warehouse := createRandomWarehouseWithOrg(t, organization)
	otherProductVariant := createRandomProductVariantWithOrg(t, otherOrganization)

	_, err := testStore.UpsertInventory(t.Context(), db.UpsertInventoryParams{
		OrganizationID:   organization.ID,
		ProductVariantID: otherProductVariant.ID,
		WarehouseID:      warehouse.ID,
		QuantityOnHand:   20,
		IsActive:         true,
	})
	require.ErrorIs(t, err, db.ErrNotFound)
}

func TestInventoryScopedQueries(t *testing.T) {
	organization := createRandomOrganization(t)
	cleanupOrganization(t, organization.ID.String())
	warehouse := createRandomWarehouseWithOrg(t, organization)
	productVariant := createRandomProductVariantWithOrg(t, organization)
	_, err := testStore.UpsertInventory(t.Context(), db.UpsertInventoryParams{
		OrganizationID:   organization.ID,
		ProductVariantID: productVariant.ID,
		WarehouseID:      warehouse.ID,
		QuantityOnHand:   12,
		IsActive:         true,
	})
	require.NoError(t, err)

	organizationRows, err := testStore.ListInventoryByOrganization(
		t.Context(),
		db.ListInventoryByOrganizationParams{
			OrganizationID: organization.ID,
			PageLimit:      20,
		},
	)
	require.NoError(t, err)
	require.Len(t, organizationRows, 1)

	productRows, err := testStore.ListInventoryByProduct(
		t.Context(),
		db.ListInventoryByProductParams{
			OrganizationID: organization.ID,
			ProductID:      productVariant.ProductID,
		},
	)
	require.NoError(t, err)
	require.Len(t, productRows, 1)
	require.Equal(t, productVariant.ID, productRows[0].ProductVariantID)

	variantRows, err := testStore.ListInventoryByVariant(
		t.Context(),
		db.ListInventoryByVariantParams{
			OrganizationID:   organization.ID,
			ProductVariantID: productVariant.ID,
		},
	)
	require.NoError(t, err)
	require.Len(t, variantRows, 1)
	require.Equal(t, warehouse.ID, variantRows[0].WarehouseID)

	detail, err := testStore.GetInventoryByVariantAndWarehouseForOrganization(
		t.Context(),
		db.GetInventoryByVariantAndWarehouseForOrganizationParams{
			OrganizationID:   organization.ID,
			ProductVariantID: productVariant.ID,
			WarehouseID:      warehouse.ID,
		},
	)
	require.NoError(t, err)
	require.Equal(t, productVariant.ProductID, detail.ProductID)
	require.Equal(t, productVariant.Sku, detail.ProductVariantSku)
}

func createCheckoutReservationTestRows(t *testing.T, orderItemQuantity int32) (db.InventoryReservation, db.OrderItem, db.ProductVariant, db.Warehouse) {
	t.Helper()

	buyerOrg, err := testStore.CreateOrganization(t.Context(), db.CreateOrganizationParams{
		Name:       "buyer-" + util.GetRandomString(t, 8),
		Type:       string(util.OrganizationTypeIndividual),
		Capability: string(util.OrganizationCapabilityBuyer),
		Slug:       "buyer-" + util.GetRandomString(t, 8),
		Status:     string(util.OrganizationStatusActive),
	})
	require.NoError(t, err)
	cleanupOrganization(t, buyerOrg.ID.String())

	sellerOrg, err := testStore.CreateOrganization(t.Context(), db.CreateOrganizationParams{
		Name:       "seller-" + util.GetRandomString(t, 8),
		Type:       string(util.OrganizationTypeMerchant),
		Capability: string(util.OrganizationCapabilitySeller),
		Slug:       "seller-" + util.GetRandomString(t, 8),
		Status:     string(util.OrganizationStatusActive),
	})
	require.NoError(t, err)
	cleanupOrganization(t, sellerOrg.ID.String())

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

	zero := decimal.Zero
	checkoutSession, err := testStore.CreateCheckoutSession(t.Context(), db.CreateCheckoutSessionParams{
		BuyerCustomerID:     customer.ID,
		BuyerOrgID:          buyerOrg.ID,
		BuyerMemberID:       member.ID,
		CheckoutFingerprint: "inventory-test-checkout",
		Subtotal:            zero,
		TaxTotal:            zero,
		ShippingTotal:       zero,
		DiscountTotal:       zero,
		GrandTotal:          zero,
		Currency:            "MYR",
	})
	require.NoError(t, err)

	productVariant := createRandomProductVariantWithOrg(t, sellerOrg)
	warehouse := createRandomWarehouseWithOrg(t, sellerOrg)

	order, err := testStore.CreateOrder(t.Context(), db.CreateOrderParams{
		CheckoutSessionID:       checkoutSession.ID,
		MerchantOrgID:           sellerOrg.ID,
		BuyerCustomerID:         customer.ID,
		BuyerOrgID:              buyerOrg.ID,
		BuyerMemberID:           member.ID,
		OrderNumber:             "ord-" + util.GetRandomString(t, 12),
		CustomerEmail:           customer.Email,
		CustomerName:            customer.Name,
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

	productID := productVariant.ProductID
	productVariantID := productVariant.ID
	orderItem, err := testStore.CreateOrderItem(t.Context(), db.CreateOrderItemParams{
		OrderID:          order.ID,
		ProductID:        &productID,
		ProductVariantID: &productVariantID,
		WarehouseID: pgtype.Int8{
			Int64: warehouse.ID,
			Valid: true,
		},
		ProductName:   "product snapshot",
		VariantName:   "variant snapshot",
		Sku:           productVariant.Sku,
		Quantity:      orderItemQuantity,
		UnitPrice:     zero,
		Subtotal:      zero,
		DiscountTotal: zero,
		TaxTotal:      zero,
		Total:         zero,
		Currency:      "MYR",
	})
	require.NoError(t, err)

	reservation, err := testStore.CreateInventoryReservation(t.Context(), db.CreateInventoryReservationParams{
		CheckoutSessionID: checkoutSession.ID,
		OrderID:           order.ID,
		BuyerOrgID:        buyerOrg.ID,
		MerchantOrgID:     sellerOrg.ID,
		ExpiresAt:         time.Now().Add(15 * time.Minute),
	})
	require.NoError(t, err)

	return reservation, orderItem, productVariant, warehouse
}

func TestInventoryReservationItemRejectsOverReservation(t *testing.T) {
	reservation, orderItem, productVariant, warehouse := createCheckoutReservationTestRows(t, 1)

	_, err := testStore.CreateInventoryReservationItem(t.Context(), db.CreateInventoryReservationItemParams{
		ReservationID:    reservation.ID,
		OrderItemID:      orderItem.ID,
		ProductVariantID: productVariant.ID,
		WarehouseID:      warehouse.ID,
		Quantity:         2,
	})
	require.Error(t, err)
}

func TestInventoryReservationItemRejectsQuantityUpdateOverReservation(t *testing.T) {
	reservation, orderItem, productVariant, warehouse := createCheckoutReservationTestRows(t, 1)

	reservationItem, err := testStore.CreateInventoryReservationItem(t.Context(), db.CreateInventoryReservationItemParams{
		ReservationID:    reservation.ID,
		OrderItemID:      orderItem.ID,
		ProductVariantID: productVariant.ID,
		WarehouseID:      warehouse.ID,
		Quantity:         1,
	})
	require.NoError(t, err)

	_, err = testPool.Exec(
		t.Context(),
		"UPDATE inventory_reservation_items SET quantity = $1 WHERE id = $2",
		2,
		reservationItem.ID,
	)
	require.Error(t, err)
}

func TestInventoryReservationMutations(t *testing.T) {
	organization := createRandomOrganization(t)
	cleanupOrganization(t, organization.ID.String())
	otherOrganization := createRandomOrganization(t)
	cleanupOrganization(t, otherOrganization.ID.String())

	warehouse := createRandomWarehouseWithOrg(t, organization)
	productVariant := createRandomProductVariantWithOrg(t, organization)
	_, err := testStore.UpsertInventory(t.Context(), db.UpsertInventoryParams{
		OrganizationID:   organization.ID,
		ProductVariantID: productVariant.ID,
		WarehouseID:      warehouse.ID,
		QuantityOnHand:   10,
		IsActive:         true,
	})
	require.NoError(t, err)

	reserved, err := testStore.ReserveInventoryForCheckout(t.Context(), db.ReserveInventoryForCheckoutParams{
		ProductVariantID: productVariant.ID,
		WarehouseID:      warehouse.ID,
		MerchantOrgID:    organization.ID,
		Quantity:         4,
	})
	require.NoError(t, err)
	require.Equal(t, int32(4), reserved.QuantityReserved)
	require.Equal(t, int32(6), *reserved.QuantityAvailable)

	released, err := testStore.ReleaseReservedInventory(t.Context(), db.ReleaseReservedInventoryParams{
		ProductVariantID: productVariant.ID,
		WarehouseID:      warehouse.ID,
		MerchantOrgID:    organization.ID,
		Quantity:         2,
	})
	require.NoError(t, err)
	require.Equal(t, int32(2), released.QuantityReserved)
	require.Equal(t, int32(8), *released.QuantityAvailable)

	confirmed, err := testStore.ConfirmReservedInventory(t.Context(), db.ConfirmReservedInventoryParams{
		ProductVariantID: productVariant.ID,
		WarehouseID:      warehouse.ID,
		MerchantOrgID:    organization.ID,
		Quantity:         2,
	})
	require.NoError(t, err)
	require.Equal(t, int32(8), confirmed.QuantityOnHand)
	require.Equal(t, int32(0), confirmed.QuantityReserved)
	require.Equal(t, int32(8), *confirmed.QuantityAvailable)

	_, err = testStore.ReserveInventoryForCheckout(t.Context(), db.ReserveInventoryForCheckoutParams{
		ProductVariantID: productVariant.ID,
		WarehouseID:      warehouse.ID,
		MerchantOrgID:    otherOrganization.ID,
		Quantity:         1,
	})
	require.ErrorIs(t, err, db.ErrNotFound)

	_, err = testStore.ReserveInventoryForCheckout(t.Context(), db.ReserveInventoryForCheckoutParams{
		ProductVariantID: productVariant.ID,
		WarehouseID:      warehouse.ID,
		MerchantOrgID:    organization.ID,
		Quantity:         0,
	})
	require.ErrorIs(t, err, db.ErrNotFound)
}
