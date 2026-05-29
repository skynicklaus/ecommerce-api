package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
)

type PaymentTransactionResponse struct {
	ID          uuid.UUID       `json:"id"`
	PaymentID   uuid.UUID       `json:"paymentId"`
	Type        string          `json:"type"`
	Status      string          `json:"status"`
	Provider    string          `json:"provider"`
	ProviderRef *string         `json:"providerRef,omitempty"`
	Amount      decimal.Decimal `json:"amount"`
	Currency    string          `json:"currency"`
	ProcessedAt *time.Time      `json:"processedAt,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type ConfirmPaymentResponse struct {
	CheckoutSession           CheckoutSessionResponse                    `json:"checkoutSession"`
	Orders                    []CheckoutOrderResponse                    `json:"orders"`
	OrderItems                []CheckoutOrderItemResponse                `json:"orderItems"`
	InventoryReservations     []CheckoutInventoryReservationResponse     `json:"inventoryReservations"`
	InventoryReservationItems []CheckoutInventoryReservationItemResponse `json:"inventoryReservationItems"`
	Payment                   CheckoutPaymentResponse                    `json:"payment"`
	PaymentTransaction        PaymentTransactionResponse                 `json:"paymentTransaction"`
}

// ConfirmManualPayment godoc
//
//	@Summary		Confirm manual payment
//	@Description	Confirms a pending manual payment and completes the reserved checkout.
//	@Tags			payments
//	@Produce		json
//	@Param			id	path		string	true	"Payment UUID"
//	@Success		200	{object}	ConfirmPaymentResponse
//	@Failure		400	{object}	apierror.APIError
//	@Failure		401	{object}	apierror.APIError
//	@Failure		403	{object}	apierror.APIError
//	@Failure		404	{object}	apierror.APIError
//	@Failure		409	{object}	apierror.APIError
//	@Failure		500	{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/payments/{id}/confirm [post]
func (h *V1Handler) ConfirmManualPayment(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	buyerOrgID, err := buyerOrgIDFromCtx(ctx)
	if err != nil {
		return err
	}
	member, err := h.buyerMemberFromCtx(ctx, buyerOrgID)
	if err != nil {
		return err
	}
	paymentID, err := parseIDParam(r, "invalid payment id")
	if err != nil {
		return err
	}

	result, err := h.store.ConfirmManualPaymentTx(ctx, db.ConfirmManualPaymentTxParams{
		PaymentID:     paymentID,
		BuyerOrgID:    buyerOrgID,
		BuyerMemberID: member.ID,
	})
	if err != nil {
		return mapPaymentConfirmationError(err)
	}

	return WriteJSON(w, http.StatusOK, confirmPaymentResponse(result))
}

func mapPaymentConfirmationError(err error) error {
	if errors.Is(err, db.ErrNotFound) {
		return newNotFoundAPIError("payment resource not found")
	}
	if errors.Is(err, db.ErrInvalidPaymentState) {
		return newConflictAPIError("payment cannot be confirmed")
	}
	if errors.Is(err, db.ErrInvalidCheckoutState) {
		return newConflictAPIError("checkout cannot be completed")
	}
	if errors.Is(err, db.ErrInvalidInventoryState) {
		return newConflictAPIError("inventory reservation cannot be confirmed")
	}
	return fmt.Errorf("payment confirmation failed: %w", err)
}

func confirmPaymentResponse(result db.ConfirmManualPaymentTxResult) ConfirmPaymentResponse {
	return ConfirmPaymentResponse{
		CheckoutSession:           checkoutSessionResponse(result.CheckoutSession),
		Orders:                    checkoutOrderResponses(result.Orders),
		OrderItems:                checkoutOrderItemResponses(result.OrderItems),
		InventoryReservations:     checkoutInventoryReservationResponses(result.InventoryReservations),
		InventoryReservationItems: checkoutInventoryReservationItemResponses(result.InventoryReservationItems),
		Payment:                   checkoutPaymentResponse(result.Payment),
		PaymentTransaction:        paymentTransactionResponse(result.PaymentTransaction),
	}
}

func paymentTransactionResponse(transaction db.PaymentTransaction) PaymentTransactionResponse {
	return PaymentTransactionResponse{
		ID:          transaction.ID,
		PaymentID:   transaction.PaymentID,
		Type:        transaction.Type,
		Status:      transaction.Status,
		Provider:    transaction.Provider,
		ProviderRef: transaction.ProviderRef,
		Amount:      transaction.Amount,
		Currency:    transaction.Currency,
		ProcessedAt: transaction.ProcessedAt,
		CreatedAt:   transaction.CreatedAt,
	}
}

func newNotFoundAPIError(message string) error {
	return apiError(http.StatusNotFound, message)
}

func newConflictAPIError(message string) error {
	return apiError(http.StatusConflict, message)
}

func apiError(status int, message string) error {
	return apierror.NewAPIError(status, errors.New(message))
}
