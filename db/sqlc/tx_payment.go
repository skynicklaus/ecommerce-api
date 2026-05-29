package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/skynicklaus/ecommerce-api/util"
)

type ConfirmManualPaymentTxParams struct {
	PaymentID     uuid.UUID
	BuyerOrgID    uuid.UUID
	BuyerMemberID uuid.UUID
	ProviderRef   *string
	Metadata      []byte
}

type ConfirmManualPaymentTxResult struct {
	CheckoutSession           CheckoutSession
	Orders                    []Order
	OrderItems                []OrderItem
	InventoryReservations     []InventoryReservation
	InventoryReservationItems []InventoryReservationItem
	Payment                   Payment
	PaymentTransaction        PaymentTransaction
	AlreadyExisted            bool
}

func (store *SQLStore) ConfirmManualPaymentTx(
	ctx context.Context,
	arg ConfirmManualPaymentTxParams,
) (ConfirmManualPaymentTxResult, error) {
	var result ConfirmManualPaymentTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		payment, err := q.GetPaymentByID(ctx, arg.PaymentID)
		if err != nil {
			return err
		}
		if payment.BuyerOrgID != arg.BuyerOrgID {
			return ErrNotFound
		}

		checkoutSession, err := q.GetCheckoutSessionForUpdate(ctx, payment.CheckoutSessionID)
		if err != nil {
			return err
		}
		if checkoutSession.BuyerOrgID != arg.BuyerOrgID {
			return ErrNotFound
		}
		if checkoutSession.BuyerMemberID != arg.BuyerMemberID {
			return ErrNotFound
		}

		payment, err = q.GetPaymentForUpdate(ctx, arg.PaymentID)
		if err != nil {
			return err
		}
		if payment.BuyerOrgID != arg.BuyerOrgID || payment.CheckoutSessionID != checkoutSession.ID {
			return ErrNotFound
		}
		if payment.Provider != string(util.PaymentProviderManual) {
			return fmt.Errorf("%w: payment %s provider is %s", ErrInvalidPaymentState, payment.ID, payment.Provider)
		}

		if payment.Status == string(util.PaymentStatusSucceeded) && checkoutSession.Status == string(util.CheckoutSessionStatusCompleted) {
			loaded, loadErr := loadConfirmManualPaymentTxResult(ctx, q, checkoutSession, payment, true)
			if loadErr != nil {
				return loadErr
			}
			result = loaded
			return nil
		}
		if !paymentCanBeConfirmed(payment.Status) {
			return fmt.Errorf("%w: payment %s is %s", ErrInvalidPaymentState, payment.ID, payment.Status)
		}
		if !checkoutCanBeCompleted(checkoutSession.Status) {
			return fmt.Errorf("%w: checkout %s is %s", ErrInvalidCheckoutState, checkoutSession.ID, checkoutSession.Status)
		}
		now := time.Now()
		if checkoutSession.ExpiresAt != nil && !checkoutSession.ExpiresAt.After(now) {
			return fmt.Errorf("%w: checkout %s expired", ErrInvalidCheckoutState, checkoutSession.ID)
		}

		reservations, err := q.ListInventoryReservationsByCheckoutSession(ctx, checkoutSession.ID)
		if err != nil {
			return err
		}
		reservationItems, err := q.ListInventoryReservationItemsByCheckoutSession(ctx, checkoutSession.ID)
		if err != nil {
			return err
		}

		reservationsByID := make(map[uuid.UUID]InventoryReservation, len(reservations))
		for _, reservation := range reservations {
			reservationsByID[reservation.ID] = reservation
			if reservation.Status == string(util.InventoryReservationStatusActive) && !reservation.ExpiresAt.After(now) {
				return fmt.Errorf("%w: reservation %s expired", ErrInvalidInventoryState, reservation.ID)
			}
		}
		for _, item := range reservationItems {
			reservation, ok := reservationsByID[item.ReservationID]
			if !ok {
				return fmt.Errorf("%w: reservation item %s has no reservation", ErrInvalidInventoryState, item.ID)
			}
			if reservation.Status != string(util.InventoryReservationStatusActive) {
				continue
			}
			if _, err = q.ConfirmReservedInventory(ctx, ConfirmReservedInventoryParams{
				Quantity:         item.Quantity,
				ProductVariantID: item.ProductVariantID,
				WarehouseID:      item.WarehouseID,
				MerchantOrgID:    reservation.MerchantOrgID,
			}); err != nil {
				if errors.Is(err, ErrNotFound) {
					return fmt.Errorf("%w: unable to confirm reserved inventory for item %s", ErrInvalidInventoryState, item.ID)
				}
				return err
			}
		}

		result.InventoryReservations = make([]InventoryReservation, 0, len(reservations))
		for _, reservation := range reservations {
			if reservation.Status != string(util.InventoryReservationStatusActive) {
				result.InventoryReservations = append(result.InventoryReservations, reservation)
				continue
			}
			confirmed, confirmErr := q.ConfirmInventoryReservation(ctx, reservation.ID)
			if confirmErr != nil {
				if errors.Is(confirmErr, ErrNotFound) {
					return fmt.Errorf("%w: reservation %s is %s", ErrInvalidInventoryState, reservation.ID, reservation.Status)
				}
				return confirmErr
			}
			result.InventoryReservations = append(result.InventoryReservations, confirmed)
		}

		payment, err = q.MarkPaymentSucceeded(ctx, MarkPaymentSucceededParams{
			PaymentID: arg.PaymentID,
			Metadata:  arg.Metadata,
		})
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return fmt.Errorf("%w: payment %s is not confirmable", ErrInvalidPaymentState, arg.PaymentID)
			}
			return err
		}
		result.Payment = payment

		processedAt := time.Now()
		result.PaymentTransaction, err = q.CreatePaymentTransaction(ctx, CreatePaymentTransactionParams{
			PaymentID:      payment.ID,
			Type:           string(util.PaymentTransactionTypeSale),
			Status:         string(util.PaymentTransactionStatusSucceeded),
			Provider:       payment.Provider,
			ProviderRef:    arg.ProviderRef,
			Amount:         payment.Amount,
			Currency:       payment.Currency,
			FailureCode:    nil,
			FailureMessage: nil,
			Metadata:       arg.Metadata,
			ProcessedAt:    &processedAt,
		})
		if err != nil {
			return err
		}

		result.Orders, err = q.MarkOrdersPlacedPaidByCheckoutSession(ctx, checkoutSession.ID)
		if err != nil {
			return err
		}
		if len(result.Orders) == 0 {
			return fmt.Errorf("%w: checkout %s has no pending orders", ErrInvalidCheckoutState, checkoutSession.ID)
		}

		result.CheckoutSession, err = q.CompleteCheckoutSession(ctx, checkoutSession.ID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return fmt.Errorf("%w: checkout %s is not completable", ErrInvalidCheckoutState, checkoutSession.ID)
			}
			return err
		}

		result.OrderItems, err = q.ListOrderItemsByCheckoutSession(ctx, checkoutSession.ID)
		if err != nil {
			return err
		}
		result.InventoryReservationItems = reservationItems

		return nil
	})

	return result, err
}

func paymentCanBeConfirmed(status string) bool {
	switch status {
	case string(util.PaymentStatusPending), string(util.PaymentStatusRequiresAction), string(util.PaymentStatusAuthorized):
		return true
	default:
		return false
	}
}

func checkoutCanBeCompleted(status string) bool {
	switch status {
	case string(util.CheckoutSessionStatusReserved), string(util.CheckoutSessionStatusPaymentPending):
		return true
	default:
		return false
	}
}

func loadConfirmManualPaymentTxResult(
	ctx context.Context,
	q *Queries,
	checkoutSession CheckoutSession,
	payment Payment,
	alreadyExisted bool,
) (ConfirmManualPaymentTxResult, error) {
	result := ConfirmManualPaymentTxResult{
		CheckoutSession: checkoutSession,
		Payment:         payment,
		AlreadyExisted:  alreadyExisted,
	}
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
	transactions, err := q.ListPaymentTransactionsByPayment(ctx, payment.ID)
	if err != nil {
		return result, err
	}
	if len(transactions) > 0 {
		result.PaymentTransaction = transactions[len(transactions)-1]
	}

	return result, nil
}
