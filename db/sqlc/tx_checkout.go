package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/skynicklaus/ecommerce-api/util"
)

const defaultCheckoutReservationTTL = 30 * time.Minute

type CheckoutSelectedCartItemsTxParams struct {
	BuyerCustomerID         uuid.UUID
	BuyerOrgID              uuid.UUID
	BuyerMemberID           uuid.UUID
	IdempotencyKey          *string
	CustomerEmail           string
	CustomerName            string
	BillingAddressSnapshot  []byte
	ShippingAddressSnapshot []byte
	Currency                string
	ReservationTTL          time.Duration
	PaymentProvider         string
}

type CheckoutSelectedCartItemsTxResult struct {
	CheckoutSession           CheckoutSession
	Orders                    []Order
	OrderItems                []OrderItem
	InventoryReservations     []InventoryReservation
	InventoryReservationItems []InventoryReservationItem
	Payment                   Payment
	DeletedCartItems          []DeleteSelectedCartItemsForCheckoutRow
	AlreadyExisted            bool
}

func (store *SQLStore) CheckoutSelectedCartItemsTx(
	ctx context.Context,
	arg CheckoutSelectedCartItemsTxParams,
) (CheckoutSelectedCartItemsTxResult, error) {
	var result CheckoutSelectedCartItemsTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		if arg.IdempotencyKey != nil {
			loaded, found, err := loadCheckoutTxResultByIdempotencyKey(ctx, q, arg)
			if err != nil {
				return err
			}
			if found {
				result = loaded
				result.AlreadyExisted = true
				return nil
			}
		}

		selectedItems, err := q.ListSelectedCartItemsForCheckout(ctx, arg.BuyerOrgID)
		if err != nil {
			return err
		}
		for _, item := range selectedItems {
			if !item.VariantIsActive {
				return fmt.Errorf("%w: product variant %s is inactive", ErrUnavailableCheckoutItem, item.ProductVariantID)
			}
			if item.ProductStatus != string(util.ProductStatusActive) {
				return fmt.Errorf("%w: product %s is not active", ErrUnavailableCheckoutItem, item.ProductID)
			}
		}

		currency := arg.Currency
		if currency == "" {
			currency = "MYR"
		}
		paymentProvider := arg.PaymentProvider
		if paymentProvider == "" {
			paymentProvider = string(util.PaymentProviderManual)
		}
		if paymentProvider != string(util.PaymentProviderManual) {
			return fmt.Errorf("%w: %s", ErrUnsupportedPaymentProvider, paymentProvider)
		}

		if len(selectedItems) == 0 {
			activeCheckout, found, err := loadActiveCheckoutSessionForBuyerForUpdate(ctx, q, arg)
			if err != nil {
				return err
			}
			if found {
				loaded, loadErr := loadCheckoutTxResult(ctx, q, activeCheckout)
				if loadErr != nil {
					return loadErr
				}
				result = loaded
				result.AlreadyExisted = true
				return nil
			}
			return fmt.Errorf("%w: checkout requires at least one selected cart item", ErrEmptyCheckout)
		}

		checkoutFingerprint, err := fingerprintCheckoutSelection(selectedItems, arg, currency, paymentProvider)
		if err != nil {
			return err
		}
		activeCheckout, found, err := loadActiveCheckoutSessionForBuyerForUpdate(ctx, q, arg)
		if err != nil {
			return err
		}
		if found {
			if activeCheckout.CheckoutFingerprint == checkoutFingerprint {
				loaded, loadErr := loadCheckoutTxResult(ctx, q, activeCheckout)
				if loadErr != nil {
					return loadErr
				}
				result = loaded
				result.AlreadyExisted = true
				return nil
			}
			if _, err = releaseCheckoutReservations(ctx, q, activeCheckout, CheckoutReleaseActionCancel, false); err != nil {
				return err
			}
		}

		reservationTTL := arg.ReservationTTL
		if reservationTTL <= 0 {
			reservationTTL = defaultCheckoutReservationTTL
		}
		expiresAt := time.Now().Add(reservationTTL)

		subtotal := decimal.Zero
		merchantSubtotals := make(map[uuid.UUID]decimal.Decimal)
		merchantItems := make(map[uuid.UUID][]ListSelectedCartItemsForCheckoutRow)
		cartItemIDs := make([]uuid.UUID, 0, len(selectedItems))

		for _, item := range selectedItems {
			lineSubtotal := item.CurrentUnitPrice.Mul(decimal.NewFromInt32(int32(item.Quantity)))
			subtotal = subtotal.Add(lineSubtotal)
			merchantSubtotals[item.MerchantOrgID] = merchantSubtotals[item.MerchantOrgID].Add(lineSubtotal)
			merchantItems[item.MerchantOrgID] = append(merchantItems[item.MerchantOrgID], item)
			cartItemIDs = append(cartItemIDs, item.CartItemID)
		}

		result.CheckoutSession, err = q.CreateCheckoutSession(ctx, CreateCheckoutSessionParams{
			BuyerCustomerID:     arg.BuyerCustomerID,
			BuyerOrgID:          arg.BuyerOrgID,
			BuyerMemberID:       arg.BuyerMemberID,
			IdempotencyKey:      arg.IdempotencyKey,
			CheckoutFingerprint: checkoutFingerprint,
			Subtotal:            subtotal,
			TaxTotal:            decimal.Zero,
			ShippingTotal:       decimal.Zero,
			DiscountTotal:       decimal.Zero,
			GrandTotal:          subtotal,
			Currency:            currency,
			ExpiresAt:           &expiresAt,
		})
		if err != nil {
			return err
		}

		for merchantOrgID, items := range merchantItems {
			order, orderErr := q.CreateOrder(ctx, CreateOrderParams{
				CheckoutSessionID:       result.CheckoutSession.ID,
				MerchantOrgID:           merchantOrgID,
				BuyerCustomerID:         arg.BuyerCustomerID,
				BuyerOrgID:              arg.BuyerOrgID,
				BuyerMemberID:           arg.BuyerMemberID,
				OrderNumber:             "ord-" + uuid.NewString(),
				CustomerEmail:           arg.CustomerEmail,
				CustomerName:            arg.CustomerName,
				BillingAddressSnapshot:  arg.BillingAddressSnapshot,
				ShippingAddressSnapshot: arg.ShippingAddressSnapshot,
				Subtotal:                merchantSubtotals[merchantOrgID],
				TaxTotal:                decimal.Zero,
				ShippingTotal:           decimal.Zero,
				ShippingDiscount:        decimal.Zero,
				CouponDiscount:          decimal.Zero,
				GrandTotal:              merchantSubtotals[merchantOrgID],
				Currency:                currency,
			})
			if orderErr != nil {
				return orderErr
			}
			result.Orders = append(result.Orders, order)

			reservation, reservationErr := q.CreateInventoryReservation(ctx, CreateInventoryReservationParams{
				CheckoutSessionID: result.CheckoutSession.ID,
				OrderID:           order.ID,
				BuyerOrgID:        arg.BuyerOrgID,
				MerchantOrgID:     merchantOrgID,
				ExpiresAt:         expiresAt,
			})
			if reservationErr != nil {
				return reservationErr
			}
			result.InventoryReservations = append(result.InventoryReservations, reservation)

			for _, item := range items {
				if err = createCheckoutOrderItemAndReservation(ctx, q, order, reservation, item, currency); err != nil {
					return err
				}
			}
		}

		result.OrderItems, err = q.ListOrderItemsByCheckoutSession(ctx, result.CheckoutSession.ID)
		if err != nil {
			return err
		}
		result.InventoryReservationItems, err = q.ListInventoryReservationItemsByCheckoutSession(ctx, result.CheckoutSession.ID)
		if err != nil {
			return err
		}

		result.Payment, err = q.CreatePayment(ctx, CreatePaymentParams{
			CheckoutSessionID: result.CheckoutSession.ID,
			BuyerOrgID:        arg.BuyerOrgID,
			Provider:          paymentProvider,
			Amount:            subtotal,
			Currency:          currency,
		})
		if err != nil {
			return err
		}

		result.DeletedCartItems, err = q.DeleteSelectedCartItemsForCheckout(ctx, DeleteSelectedCartItemsForCheckoutParams{
			BuyerOrgID:  arg.BuyerOrgID,
			CartItemIds: cartItemIDs,
		})
		if err != nil {
			return err
		}
		cartIDs := make(map[uuid.UUID]struct{}, len(selectedItems))
		for _, item := range selectedItems {
			cartIDs[item.CartID] = struct{}{}
		}
		for cartID := range cartIDs {
			if err = q.DeleteEmptyCartShopGroups(ctx, cartID); err != nil {
				return err
			}
		}

		result.CheckoutSession, err = q.MarkCheckoutSessionReserved(ctx, result.CheckoutSession.ID)
		return err
	})

	return result, err
}

func loadCheckoutTxResultByIdempotencyKey(
	ctx context.Context,
	q *Queries,
	arg CheckoutSelectedCartItemsTxParams,
) (CheckoutSelectedCartItemsTxResult, bool, error) {
	checkoutSession, err := q.GetCheckoutSessionByIdempotencyKey(ctx, GetCheckoutSessionByIdempotencyKeyParams{
		BuyerOrgID:     arg.BuyerOrgID,
		BuyerMemberID:  arg.BuyerMemberID,
		IdempotencyKey: *arg.IdempotencyKey,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CheckoutSelectedCartItemsTxResult{}, false, nil
		}
		return CheckoutSelectedCartItemsTxResult{}, false, err
	}

	result, err := loadCheckoutTxResult(ctx, q, checkoutSession)
	return result, true, err
}

func loadActiveCheckoutSessionForBuyerForUpdate(
	ctx context.Context,
	q *Queries,
	arg CheckoutSelectedCartItemsTxParams,
) (CheckoutSession, bool, error) {
	checkoutSession, err := q.GetActiveCheckoutSessionForBuyerForUpdate(ctx, GetActiveCheckoutSessionForBuyerForUpdateParams{
		BuyerOrgID:    arg.BuyerOrgID,
		BuyerMemberID: arg.BuyerMemberID,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return CheckoutSession{}, false, nil
		}
		return CheckoutSession{}, false, err
	}

	return checkoutSession, true, nil
}

type checkoutSelectionFingerprint struct {
	BuyerOrgID              uuid.UUID                          `json:"buyerOrgId"`
	BuyerMemberID           uuid.UUID                          `json:"buyerMemberId"`
	Currency                string                             `json:"currency"`
	PaymentProvider         string                             `json:"paymentProvider"`
	BillingAddressSnapshot  json.RawMessage                    `json:"billingAddressSnapshot,omitempty"`
	ShippingAddressSnapshot json.RawMessage                    `json:"shippingAddressSnapshot"`
	Items                   []checkoutSelectionFingerprintItem `json:"items"`
}

type checkoutSelectionFingerprintItem struct {
	MerchantOrgID    uuid.UUID `json:"merchantOrgId"`
	ProductVariantID uuid.UUID `json:"productVariantId"`
	Quantity         int16     `json:"quantity"`
	CurrentUnitPrice string    `json:"currentUnitPrice"`
	TrackInventory   bool      `json:"trackInventory"`
}

func fingerprintCheckoutSelection(
	selectedItems []ListSelectedCartItemsForCheckoutRow,
	arg CheckoutSelectedCartItemsTxParams,
	currency string,
	paymentProvider string,
) (string, error) {
	items := make([]checkoutSelectionFingerprintItem, len(selectedItems))
	for i, item := range selectedItems {
		items[i] = checkoutSelectionFingerprintItem{
			MerchantOrgID:    item.MerchantOrgID,
			ProductVariantID: item.ProductVariantID,
			Quantity:         item.Quantity,
			CurrentUnitPrice: item.CurrentUnitPrice.String(),
			TrackInventory:   item.TrackInventory,
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].MerchantOrgID != items[j].MerchantOrgID {
			return items[i].MerchantOrgID.String() < items[j].MerchantOrgID.String()
		}
		return items[i].ProductVariantID.String() < items[j].ProductVariantID.String()
	})

	billingAddressSnapshot, err := normalizeCheckoutSnapshot(arg.BillingAddressSnapshot)
	if err != nil {
		return "", fmt.Errorf("normalize checkout billing address snapshot: %w", err)
	}
	shippingAddressSnapshot, err := normalizeCheckoutSnapshot(arg.ShippingAddressSnapshot)
	if err != nil {
		return "", fmt.Errorf("normalize checkout shipping address snapshot: %w", err)
	}

	payload := checkoutSelectionFingerprint{
		BuyerOrgID:              arg.BuyerOrgID,
		BuyerMemberID:           arg.BuyerMemberID,
		Currency:                currency,
		PaymentProvider:         paymentProvider,
		BillingAddressSnapshot:  billingAddressSnapshot,
		ShippingAddressSnapshot: shippingAddressSnapshot,
		Items:                   items,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal checkout fingerprint: %w", err)
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}

func normalizeCheckoutSnapshot(snapshot []byte) (json.RawMessage, error) {
	if len(snapshot) == 0 {
		return nil, nil
	}

	var value any
	decoder := json.NewDecoder(bytes.NewReader(snapshot))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}

	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(normalized), nil
}

func createCheckoutOrderItemAndReservation(
	ctx context.Context,
	q *Queries,
	order Order,
	reservation InventoryReservation,
	item ListSelectedCartItemsForCheckoutRow,
	currency string,
) error {
	quantity := int32(item.Quantity)
	var warehouseID *int64

	if item.TrackInventory {
		candidateInventories, err := q.ListInventoryCandidatesForCheckoutItem(ctx, ListInventoryCandidatesForCheckoutItemParams{
			ProductVariantID: item.ProductVariantID,
			MerchantOrgID:    item.MerchantOrgID,
			Quantity:         quantity,
			PageLimit:        1,
		})
		if err != nil {
			return err
		}
		if len(candidateInventories) == 0 {
			return fmt.Errorf("%w for product variant %s", ErrInsufficientInventory, item.ProductVariantID)
		}
		warehouseID = &candidateInventories[0].WarehouseID
	}

	lineSubtotal := item.CurrentUnitPrice.Mul(decimal.NewFromInt32(quantity))
	productID := item.ProductID
	variantID := item.ProductVariantID
	productSlug := item.ProductSlug
	thumbnailAssetKey := item.ThumbnailAssetKey
	warehouseIDParam := pgtype.Int8{}
	if warehouseID != nil {
		warehouseIDParam = pgtype.Int8{Int64: *warehouseID, Valid: true}
	}

	orderItem, err := q.CreateOrderItem(ctx, CreateOrderItemParams{
		OrderID:           order.ID,
		ProductID:         &productID,
		ProductVariantID:  &variantID,
		WarehouseID:       warehouseIDParam,
		ProductName:       item.ProductName,
		ProductSlug:       &productSlug,
		VariantName:       item.VariantName,
		Sku:               item.Sku,
		ThumbnailAssetKey: &thumbnailAssetKey,
		Quantity:          quantity,
		UnitPrice:         item.CurrentUnitPrice,
		Subtotal:          lineSubtotal,
		DiscountTotal:     decimal.Zero,
		TaxTotal:          decimal.Zero,
		Total:             lineSubtotal,
		Currency:          currency,
	})
	if err != nil {
		return err
	}

	if !item.TrackInventory {
		return nil
	}

	if _, err = q.ReserveInventoryForCheckout(ctx, ReserveInventoryForCheckoutParams{
		Quantity:         quantity,
		ProductVariantID: item.ProductVariantID,
		WarehouseID:      *warehouseID,
		MerchantOrgID:    item.MerchantOrgID,
	}); err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("%w for product variant %s", ErrInsufficientInventory, item.ProductVariantID)
		}
		return err
	}

	_, err = q.CreateInventoryReservationItem(ctx, CreateInventoryReservationItemParams{
		ReservationID:    reservation.ID,
		OrderItemID:      orderItem.ID,
		ProductVariantID: item.ProductVariantID,
		WarehouseID:      *warehouseID,
		Quantity:         quantity,
	})
	return err
}

func loadCheckoutTxResult(
	ctx context.Context,
	q *Queries,
	checkoutSession CheckoutSession,
) (CheckoutSelectedCartItemsTxResult, error) {
	result := CheckoutSelectedCartItemsTxResult{CheckoutSession: checkoutSession}
	var err error

	result.Orders, err = q.ListOrdersByCheckoutSession(ctx, checkoutSession.ID)
	if err != nil {
		return result, err
	}
	result.OrderItems, err = q.ListOrderItemsByCheckoutSession(ctx, checkoutSession.ID)
	if err != nil {
		return result, err
	}
	result.InventoryReservations, err = q.ListInventoryReservationsByCheckoutSession(ctx, checkoutSession.ID)
	if err != nil {
		return result, err
	}
	result.InventoryReservationItems, err = q.ListInventoryReservationItemsByCheckoutSession(ctx, checkoutSession.ID)
	if err != nil {
		return result, err
	}
	payments, err := q.ListPaymentsByCheckoutSession(ctx, checkoutSession.ID)
	if err != nil {
		return result, err
	}
	if len(payments) > 0 {
		result.Payment = payments[0]
	}

	return result, nil
}
