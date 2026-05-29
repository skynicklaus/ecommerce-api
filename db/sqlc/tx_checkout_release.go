package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/skynicklaus/ecommerce-api/util"
)

type CheckoutReleaseAction string

const (
	CheckoutReleaseActionCancel CheckoutReleaseAction = "cancel"
	CheckoutReleaseActionExpire CheckoutReleaseAction = "expire"
)

type ReleaseCheckoutReservationsTxParams struct {
	CheckoutSessionID uuid.UUID
	BuyerOrgID        uuid.UUID
	BuyerMemberID     uuid.UUID
	Action            CheckoutReleaseAction
}

type ReleaseCheckoutReservationsTxResult struct {
	CheckoutSession           CheckoutSession
	Orders                    []Order
	OrderItems                []OrderItem
	InventoryReservations     []InventoryReservation
	InventoryReservationItems []InventoryReservationItem
	Payment                   Payment
	AlreadyExisted            bool
}

func (store *SQLStore) ReleaseCheckoutReservationsTx(
	ctx context.Context,
	arg ReleaseCheckoutReservationsTxParams,
) (ReleaseCheckoutReservationsTxResult, error) {
	var result ReleaseCheckoutReservationsTxResult
	if arg.Action == "" {
		arg.Action = CheckoutReleaseActionCancel
	}
	if arg.Action != CheckoutReleaseActionCancel && arg.Action != CheckoutReleaseActionExpire {
		return result, fmt.Errorf("%w: unknown checkout release action %q", ErrInvalidCheckoutState, arg.Action)
	}

	err := store.execTx(ctx, func(q *Queries) error {
		checkoutSession, err := q.GetCheckoutSessionForUpdate(ctx, arg.CheckoutSessionID)
		if err != nil {
			return err
		}
		if checkoutSession.BuyerOrgID != arg.BuyerOrgID {
			return ErrNotFound
		}
		if checkoutSession.BuyerMemberID != arg.BuyerMemberID {
			return ErrNotFound
		}
		released, err := releaseCheckoutReservations(ctx, q, checkoutSession, arg.Action, false)
		if err != nil {
			return err
		}
		result = released
		return nil
	})

	return result, err
}

func releaseCheckoutReservations(
	ctx context.Context,
	q *Queries,
	checkoutSession CheckoutSession,
	action CheckoutReleaseAction,
	alreadyExisted bool,
) (ReleaseCheckoutReservationsTxResult, error) {
	var result ReleaseCheckoutReservationsTxResult
	if checkoutSession.Status == string(util.CheckoutSessionStatusCompleted) {
		return result, fmt.Errorf("%w: checkout %s is completed", ErrInvalidCheckoutState, checkoutSession.ID)
	}
	if checkoutReleaseIsTerminal(checkoutSession.Status) {
		return loadReleaseCheckoutReservationsTxResult(ctx, q, checkoutSession, true)
	}
	if !checkoutCanBeReleased(checkoutSession.Status) {
		return result, fmt.Errorf("%w: checkout %s is %s", ErrInvalidCheckoutState, checkoutSession.ID, checkoutSession.Status)
	}

	reservations, err := q.ListInventoryReservationsByCheckoutSession(ctx, checkoutSession.ID)
	if err != nil {
		return result, err
	}
	reservationItems, err := q.ListInventoryReservationItemsByCheckoutSession(ctx, checkoutSession.ID)
	if err != nil {
		return result, err
	}

	reservationsByID := make(map[uuid.UUID]InventoryReservation, len(reservations))
	for _, reservation := range reservations {
		reservationsByID[reservation.ID] = reservation
	}
	for _, item := range reservationItems {
		reservation, ok := reservationsByID[item.ReservationID]
		if !ok {
			return result, fmt.Errorf("%w: reservation item %s has no reservation", ErrInvalidInventoryState, item.ID)
		}
		if reservation.Status != string(util.InventoryReservationStatusActive) {
			continue
		}
		if _, err = q.ReleaseReservedInventory(ctx, ReleaseReservedInventoryParams{
			Quantity:         item.Quantity,
			ProductVariantID: item.ProductVariantID,
			WarehouseID:      item.WarehouseID,
			MerchantOrgID:    reservation.MerchantOrgID,
		}); err != nil {
			if errors.Is(err, ErrNotFound) {
				return result, fmt.Errorf("%w: unable to release reserved inventory for item %s", ErrInvalidInventoryState, item.ID)
			}
			return result, err
		}
	}

	result.InventoryReservations = make([]InventoryReservation, 0, len(reservations))
	for _, reservation := range reservations {
		if reservation.Status != string(util.InventoryReservationStatusActive) {
			result.InventoryReservations = append(result.InventoryReservations, reservation)
			continue
		}
		released, releaseErr := releaseInventoryReservationForCheckout(ctx, q, reservation.ID, action)
		if releaseErr != nil {
			return result, releaseErr
		}
		result.InventoryReservations = append(result.InventoryReservations, released)
	}

	orders, err := q.ListOrdersByCheckoutSession(ctx, checkoutSession.ID)
	if err != nil {
		return result, err
	}
	result.Orders = make([]Order, 0, len(orders))
	for _, order := range orders {
		switch order.Status {
		case string(util.OrderStatusPendingPayment):
			released, releaseErr := releaseOrderForCheckout(ctx, q, order.ID, action)
			if releaseErr != nil {
				return result, releaseErr
			}
			result.Orders = append(result.Orders, released)
		case string(util.OrderStatusCancelled), string(util.OrderStatusExpired):
			result.Orders = append(result.Orders, order)
		default:
			return result, fmt.Errorf("%w: order %s is %s", ErrInvalidCheckoutState, order.ID, order.Status)
		}
	}

	payments, err := q.ListPaymentsByCheckoutSession(ctx, checkoutSession.ID)
	if err != nil {
		return result, err
	}
	if len(payments) > 0 {
		payment := payments[0]
		switch payment.Status {
		case string(util.PaymentStatusPending), string(util.PaymentStatusRequiresAction), string(util.PaymentStatusAuthorized):
			result.Payment, err = q.UpdatePaymentStatus(ctx, UpdatePaymentStatusParams{
				PaymentID: payment.ID,
				Status:    string(util.PaymentStatusCancelled),
				Metadata:  nil,
			})
			if err != nil {
				return result, err
			}
		case string(util.PaymentStatusCancelled), string(util.PaymentStatusFailed):
			result.Payment = payment
		default:
			return result, fmt.Errorf("%w: payment %s is %s", ErrInvalidPaymentState, payment.ID, payment.Status)
		}
	}

	result.CheckoutSession, err = releaseCheckoutSession(ctx, q, checkoutSession.ID, action)
	if err != nil {
		return result, err
	}
	result.OrderItems, err = q.ListOrderItemsByCheckoutSession(ctx, checkoutSession.ID)
	if err != nil {
		return result, err
	}
	result.InventoryReservationItems = reservationItems
	result.AlreadyExisted = alreadyExisted

	return result, nil
}

func checkoutCanBeReleased(status string) bool {
	switch status {
	case string(util.CheckoutSessionStatusPending), string(util.CheckoutSessionStatusReserved), string(util.CheckoutSessionStatusPaymentPending):
		return true
	default:
		return false
	}
}

func checkoutReleaseIsTerminal(status string) bool {
	switch status {
	case string(util.CheckoutSessionStatusCancelled), string(util.CheckoutSessionStatusExpired):
		return true
	default:
		return false
	}
}

func releaseInventoryReservationForCheckout(
	ctx context.Context,
	q *Queries,
	reservationID uuid.UUID,
	action CheckoutReleaseAction,
) (InventoryReservation, error) {
	switch action {
	case CheckoutReleaseActionExpire:
		return q.ExpireInventoryReservation(ctx, reservationID)
	default:
		return q.CancelInventoryReservation(ctx, reservationID)
	}
}

func releaseOrderForCheckout(
	ctx context.Context,
	q *Queries,
	orderID uuid.UUID,
	action CheckoutReleaseAction,
) (Order, error) {
	switch action {
	case CheckoutReleaseActionExpire:
		return q.ExpirePendingOrder(ctx, orderID)
	default:
		return q.CancelOrder(ctx, orderID)
	}
}

func releaseCheckoutSession(
	ctx context.Context,
	q *Queries,
	checkoutSessionID uuid.UUID,
	action CheckoutReleaseAction,
) (CheckoutSession, error) {
	switch action {
	case CheckoutReleaseActionExpire:
		return q.ExpireCheckoutSession(ctx, checkoutSessionID)
	default:
		return q.CancelCheckoutSession(ctx, checkoutSessionID)
	}
}

func loadReleaseCheckoutReservationsTxResult(
	ctx context.Context,
	q *Queries,
	checkoutSession CheckoutSession,
	alreadyExisted bool,
) (ReleaseCheckoutReservationsTxResult, error) {
	result := ReleaseCheckoutReservationsTxResult{
		CheckoutSession: checkoutSession,
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
	payments, err := q.ListPaymentsByCheckoutSession(ctx, checkoutSession.ID)
	if err != nil {
		return result, err
	}
	if len(payments) > 0 {
		result.Payment = payments[0]
	}

	return result, nil
}
