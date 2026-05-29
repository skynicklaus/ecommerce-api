package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/internal/middleware"
	"github.com/skynicklaus/ecommerce-api/util"
)

const idempotencyKeyHeader = "Idempotency-Key"

type CreateCheckoutRequest struct {
	BillingAddressSnapshot  json.RawMessage `json:"billingAddress,omitempty"  swaggertype:"object"`
	ShippingAddressSnapshot json.RawMessage `json:"shippingAddress"           swaggertype:"object"`
	Currency                string          `json:"currency,omitempty"         validate:"omitempty,len=3"`
	PaymentProvider         string          `json:"paymentProvider,omitempty"`
}

type CheckoutSessionResponse struct {
	ID              uuid.UUID       `json:"id"`
	BuyerCustomerID uuid.UUID       `json:"buyerCustomerId"`
	BuyerOrgID      uuid.UUID       `json:"buyerOrgId"`
	BuyerMemberID   uuid.UUID       `json:"buyerMemberId"`
	Status          string          `json:"status"`
	Subtotal        decimal.Decimal `json:"subtotal"`
	TaxTotal        decimal.Decimal `json:"taxTotal"`
	ShippingTotal   decimal.Decimal `json:"shippingTotal"`
	DiscountTotal   decimal.Decimal `json:"discountTotal"`
	GrandTotal      decimal.Decimal `json:"grandTotal"`
	Currency        string          `json:"currency"`
	ExpiresAt       *time.Time      `json:"expiresAt,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

type CheckoutOrderResponse struct {
	ID                uuid.UUID       `json:"id"`
	MerchantOrgID     uuid.UUID       `json:"merchantOrgId"`
	OrderNumber       string          `json:"orderNumber"`
	Status            string          `json:"status"`
	PaymentStatus     string          `json:"paymentStatus"`
	FulfillmentStatus string          `json:"fulfillmentStatus"`
	Subtotal          decimal.Decimal `json:"subtotal"`
	GrandTotal        decimal.Decimal `json:"grandTotal"`
	Currency          string          `json:"currency"`
	CreatedAt         time.Time       `json:"createdAt"`
}

type CheckoutOrderItemResponse struct {
	ID                uuid.UUID       `json:"id"`
	OrderID           uuid.UUID       `json:"orderId"`
	ProductID         *uuid.UUID      `json:"productId,omitempty"`
	ProductVariantID  *uuid.UUID      `json:"productVariantId,omitempty"`
	WarehouseID       *int64          `json:"warehouseId,omitempty"`
	ProductName       string          `json:"productName"`
	ProductSlug       *string         `json:"productSlug,omitempty"`
	VariantName       string          `json:"variantName"`
	Sku               string          `json:"sku"`
	ThumbnailAssetKey *string         `json:"thumbnailAssetKey,omitempty"`
	Quantity          int32           `json:"quantity"`
	UnitPrice         decimal.Decimal `json:"unitPrice"`
	Subtotal          decimal.Decimal `json:"subtotal"`
	Total             decimal.Decimal `json:"total"`
	Currency          string          `json:"currency"`
}

type CheckoutInventoryReservationResponse struct {
	ID            uuid.UUID `json:"id"`
	OrderID       uuid.UUID `json:"orderId"`
	MerchantOrgID uuid.UUID `json:"merchantOrgId"`
	Status        string    `json:"status"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

type CheckoutInventoryReservationItemResponse struct {
	ID               uuid.UUID `json:"id"`
	ReservationID    uuid.UUID `json:"reservationId"`
	OrderItemID      uuid.UUID `json:"orderItemId"`
	ProductVariantID uuid.UUID `json:"productVariantId"`
	WarehouseID      int64     `json:"warehouseId"`
	Quantity         int32     `json:"quantity"`
}

type CheckoutPaymentResponse struct {
	ID       uuid.UUID       `json:"id"`
	Provider string          `json:"provider"`
	Status   string          `json:"status"`
	Amount   decimal.Decimal `json:"amount"`
	Currency string          `json:"currency"`
}

type CheckoutResponse struct {
	CheckoutSession           CheckoutSessionResponse                    `json:"checkoutSession"`
	Orders                    []CheckoutOrderResponse                    `json:"orders"`
	OrderItems                []CheckoutOrderItemResponse                `json:"orderItems"`
	InventoryReservations     []CheckoutInventoryReservationResponse     `json:"inventoryReservations"`
	InventoryReservationItems []CheckoutInventoryReservationItemResponse `json:"inventoryReservationItems"`
	Payment                   CheckoutPaymentResponse                    `json:"payment"`
}

// CreateCheckout godoc
//
//	@Summary		Create checkout
//	@Description	Checks out selected cart items for the authenticated buyer organization and reserves inventory.
//	@Tags			checkout
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string					false	"Optional idempotency key scoped to the buyer member"
//	@Param			request			body		CreateCheckoutRequest	true	"Checkout payload"
//	@Success		201				{object}	CheckoutResponse
//	@Success		200				{object}	CheckoutResponse
//	@Failure		400				{object}	apierror.APIError
//	@Failure		401				{object}	apierror.APIError
//	@Failure		403				{object}	apierror.APIError
//	@Failure		409				{object}	apierror.APIError
//	@Failure		422				{object}	apierror.APIError
//	@Failure		500				{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/checkout [post]
func (h *V1Handler) CreateCheckout(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	identity, err := middleware.GetIdentityFromContext(ctx)
	if err != nil {
		return apierror.NewAPIError(http.StatusUnauthorized, err)
	}
	buyerOrgID, err := buyerOrgIDFromCtx(ctx)
	if err != nil {
		return err
	}
	member, err := h.buyerMemberFromCtx(ctx, buyerOrgID)
	if err != nil {
		return err
	}

	var req CreateCheckoutRequest
	if err = decodeJSON(w, r, &req); err != nil {
		return err
	}
	if err = h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}
	if err = validateJSONObjectRaw(req.ShippingAddressSnapshot); err != nil {
		return apierror.NewAPIError(http.StatusUnprocessableEntity, errors.New("shippingAddress must be a JSON object"))
	}
	if len(req.BillingAddressSnapshot) > 0 {
		if err = validateJSONObjectRaw(req.BillingAddressSnapshot); err != nil {
			return apierror.NewAPIError(http.StatusUnprocessableEntity, errors.New("billingAddress must be a JSON object"))
		}
	}
	if req.PaymentProvider != "" && req.PaymentProvider != string(util.PaymentProviderManual) {
		return apierror.NewAPIError(http.StatusUnprocessableEntity, errors.New("unsupported payment provider"))
	}

	idempotencyKey := strings.TrimSpace(r.Header.Get(idempotencyKeyHeader))
	var idempotencyKeyPtr *string
	if idempotencyKey != "" {
		idempotencyKeyPtr = &idempotencyKey
	}

	result, err := h.store.CheckoutSelectedCartItemsTx(ctx, db.CheckoutSelectedCartItemsTxParams{
		BuyerCustomerID:         identity.ActorID,
		BuyerOrgID:              buyerOrgID,
		BuyerMemberID:           member.ID,
		IdempotencyKey:          idempotencyKeyPtr,
		CustomerEmail:           identity.Email,
		CustomerName:            identity.Name,
		BillingAddressSnapshot:  req.BillingAddressSnapshot,
		ShippingAddressSnapshot: req.ShippingAddressSnapshot,
		Currency:                req.Currency,
		PaymentProvider:         req.PaymentProvider,
	})
	if err != nil {
		return mapCheckoutError(err)
	}

	status := http.StatusCreated
	if result.AlreadyExisted {
		status = http.StatusOK
	}

	return WriteJSON(w, status, checkoutResponse(result))
}

// CancelCheckout godoc
//
//	@Summary		Cancel checkout
//	@Description	Cancels an active checkout and releases reserved inventory.
//	@Tags			checkout
//	@Produce		json
//	@Param			id	path		string	true	"Checkout session UUID"
//	@Success		200	{object}	CheckoutResponse
//	@Failure		400	{object}	apierror.APIError
//	@Failure		401	{object}	apierror.APIError
//	@Failure		403	{object}	apierror.APIError
//	@Failure		404	{object}	apierror.APIError
//	@Failure		409	{object}	apierror.APIError
//	@Failure		500	{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/checkout/{id}/cancel [post]
func (h *V1Handler) CancelCheckout(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	buyerOrgID, err := buyerOrgIDFromCtx(ctx)
	if err != nil {
		return err
	}
	member, err := h.buyerMemberFromCtx(ctx, buyerOrgID)
	if err != nil {
		return err
	}
	checkoutSessionID, err := parseIDParam(r, "invalid checkout id")
	if err != nil {
		return err
	}

	result, err := h.store.ReleaseCheckoutReservationsTx(ctx, db.ReleaseCheckoutReservationsTxParams{
		CheckoutSessionID: checkoutSessionID,
		BuyerOrgID:        buyerOrgID,
		BuyerMemberID:     member.ID,
		Action:            db.CheckoutReleaseActionCancel,
	})
	if err != nil {
		return mapCheckoutReleaseError(err)
	}

	return WriteJSON(w, http.StatusOK, checkoutReleaseResponse(result))
}

func (h *V1Handler) buyerMemberFromCtx(ctx context.Context, buyerOrgID uuid.UUID) (db.Member, error) {
	identity, err := middleware.GetIdentityFromContext(ctx)
	if err != nil {
		return db.Member{}, apierror.NewAPIError(http.StatusUnauthorized, err)
	}
	member, err := h.store.GetMemberByIdentityID(ctx, identity.IdentityID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.Member{}, apierror.NewAPIError(http.StatusForbidden, errors.New("buyer member required"))
		}
		return db.Member{}, fmt.Errorf("failed to resolve buyer member: %w", err)
	}
	if member.OrganizationID != buyerOrgID {
		return db.Member{}, apierror.NewAPIError(http.StatusForbidden, errors.New("buyer member organization mismatch"))
	}

	return member, nil
}

func validateJSONObjectRaw(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("missing JSON object")
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if value == nil {
		return errors.New("missing JSON object")
	}

	return nil
}

func mapCheckoutError(err error) error {
	if errors.Is(err, db.ErrNotFound) {
		return apierror.NewAPIError(http.StatusNotFound, errors.New("checkout resource not found"))
	}
	if db.ErrorCode(err) == db.CheckViolation {
		return apierror.NewAPIError(http.StatusUnprocessableEntity, errors.New("invalid checkout data"))
	}
	if db.ErrorCode(err) == db.ForeignKeyViolation {
		return apierror.NewAPIError(http.StatusUnprocessableEntity, errors.New("invalid checkout reference"))
	}
	if db.ErrorCode(err) == db.UniqueViolation {
		return apierror.NewAPIError(http.StatusConflict, errors.New("active checkout already exists"))
	}
	if errors.Is(err, db.ErrInsufficientInventory) {
		return apierror.NewAPIError(http.StatusConflict, errors.New("insufficient inventory"))
	}
	if errors.Is(err, db.ErrInvalidCheckoutState) {
		return apierror.NewAPIError(http.StatusConflict, errors.New("checkout cannot be replaced"))
	}
	if errors.Is(err, db.ErrUnsupportedPaymentProvider) {
		return apierror.NewAPIError(http.StatusUnprocessableEntity, errors.New("unsupported payment provider"))
	}
	if errors.Is(err, db.ErrEmptyCheckout) {
		return apierror.NewAPIError(http.StatusUnprocessableEntity, errors.New("checkout requires at least one selected cart item"))
	}
	if errors.Is(err, db.ErrUnavailableCheckoutItem) {
		return apierror.NewAPIError(http.StatusConflict, errors.New("selected cart item is no longer available"))
	}
	return fmt.Errorf("checkout operation failed: %w", err)
}

func mapCheckoutReleaseError(err error) error {
	if errors.Is(err, db.ErrNotFound) {
		return apierror.NewAPIError(http.StatusNotFound, errors.New("checkout resource not found"))
	}
	if errors.Is(err, db.ErrInvalidCheckoutState) {
		return apierror.NewAPIError(http.StatusConflict, errors.New("checkout cannot be cancelled"))
	}
	if errors.Is(err, db.ErrInvalidPaymentState) {
		return apierror.NewAPIError(http.StatusConflict, errors.New("payment cannot be cancelled"))
	}
	if errors.Is(err, db.ErrInvalidInventoryState) {
		return apierror.NewAPIError(http.StatusConflict, errors.New("inventory reservation cannot be released"))
	}
	return fmt.Errorf("checkout cancellation failed: %w", err)
}

func checkoutResponse(result db.CheckoutSelectedCartItemsTxResult) CheckoutResponse {
	return CheckoutResponse{
		CheckoutSession:           checkoutSessionResponse(result.CheckoutSession),
		Orders:                    checkoutOrderResponses(result.Orders),
		OrderItems:                checkoutOrderItemResponses(result.OrderItems),
		InventoryReservations:     checkoutInventoryReservationResponses(result.InventoryReservations),
		InventoryReservationItems: checkoutInventoryReservationItemResponses(result.InventoryReservationItems),
		Payment:                   checkoutPaymentResponse(result.Payment),
	}
}

func checkoutReleaseResponse(result db.ReleaseCheckoutReservationsTxResult) CheckoutResponse {
	return CheckoutResponse{
		CheckoutSession:           checkoutSessionResponse(result.CheckoutSession),
		Orders:                    checkoutOrderResponses(result.Orders),
		OrderItems:                checkoutOrderItemResponses(result.OrderItems),
		InventoryReservations:     checkoutInventoryReservationResponses(result.InventoryReservations),
		InventoryReservationItems: checkoutInventoryReservationItemResponses(result.InventoryReservationItems),
		Payment:                   checkoutPaymentResponse(result.Payment),
	}
}

func checkoutSessionResponse(session db.CheckoutSession) CheckoutSessionResponse {
	return CheckoutSessionResponse{
		ID:              session.ID,
		BuyerCustomerID: session.BuyerCustomerID,
		BuyerOrgID:      session.BuyerOrgID,
		BuyerMemberID:   session.BuyerMemberID,
		Status:          session.Status,
		Subtotal:        session.Subtotal,
		TaxTotal:        session.TaxTotal,
		ShippingTotal:   session.ShippingTotal,
		DiscountTotal:   session.DiscountTotal,
		GrandTotal:      session.GrandTotal,
		Currency:        session.Currency,
		ExpiresAt:       session.ExpiresAt,
		CreatedAt:       session.CreatedAt,
		UpdatedAt:       session.UpdatedAt,
	}
}

func checkoutOrderResponses(orders []db.Order) []CheckoutOrderResponse {
	resp := make([]CheckoutOrderResponse, len(orders))
	for i, order := range orders {
		resp[i] = CheckoutOrderResponse{
			ID:                order.ID,
			MerchantOrgID:     order.MerchantOrgID,
			OrderNumber:       order.OrderNumber,
			Status:            order.Status,
			PaymentStatus:     order.PaymentStatus,
			FulfillmentStatus: order.FulfillmentStatus,
			Subtotal:          order.Subtotal,
			GrandTotal:        order.GrandTotal,
			Currency:          order.Currency,
			CreatedAt:         order.CreatedAt,
		}
	}
	return resp
}

func checkoutOrderItemResponses(orderItems []db.OrderItem) []CheckoutOrderItemResponse {
	resp := make([]CheckoutOrderItemResponse, len(orderItems))
	for i, item := range orderItems {
		var warehouseID *int64
		if item.WarehouseID.Valid {
			warehouseID = &item.WarehouseID.Int64
		}
		resp[i] = CheckoutOrderItemResponse{
			ID:                item.ID,
			OrderID:           item.OrderID,
			ProductID:         item.ProductID,
			ProductVariantID:  item.ProductVariantID,
			WarehouseID:       warehouseID,
			ProductName:       item.ProductName,
			ProductSlug:       item.ProductSlug,
			VariantName:       item.VariantName,
			Sku:               item.Sku,
			ThumbnailAssetKey: item.ThumbnailAssetKey,
			Quantity:          item.Quantity,
			UnitPrice:         item.UnitPrice,
			Subtotal:          item.Subtotal,
			Total:             item.Total,
			Currency:          item.Currency,
		}
	}
	return resp
}

func checkoutInventoryReservationResponses(
	reservations []db.InventoryReservation,
) []CheckoutInventoryReservationResponse {
	resp := make([]CheckoutInventoryReservationResponse, len(reservations))
	for i, reservation := range reservations {
		resp[i] = CheckoutInventoryReservationResponse{
			ID:            reservation.ID,
			OrderID:       reservation.OrderID,
			MerchantOrgID: reservation.MerchantOrgID,
			Status:        reservation.Status,
			ExpiresAt:     reservation.ExpiresAt,
		}
	}
	return resp
}

func checkoutInventoryReservationItemResponses(
	items []db.InventoryReservationItem,
) []CheckoutInventoryReservationItemResponse {
	resp := make([]CheckoutInventoryReservationItemResponse, len(items))
	for i, item := range items {
		resp[i] = CheckoutInventoryReservationItemResponse{
			ID:               item.ID,
			ReservationID:    item.ReservationID,
			OrderItemID:      item.OrderItemID,
			ProductVariantID: item.ProductVariantID,
			WarehouseID:      item.WarehouseID,
			Quantity:         item.Quantity,
		}
	}
	return resp
}

func checkoutPaymentResponse(payment db.Payment) CheckoutPaymentResponse {
	return CheckoutPaymentResponse{
		ID:       payment.ID,
		Provider: payment.Provider,
		Status:   payment.Status,
		Amount:   payment.Amount,
		Currency: payment.Currency,
	}
}
