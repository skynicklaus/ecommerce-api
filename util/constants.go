package util

import "time"

type IdentityType string

const (
	IdentityUser     IdentityType = "user"
	IdentityCustomer IdentityType = "customer"
)

type SessionService string

const (
	SessionServiceAdminPanel    SessionService = "admin_panel"
	SessionServiceMerchantPanel SessionService = "merchant_panel"
	SessionServiceBuyerPlatform SessionService = "buyer_platform"
)

// SessionTTL is the lifetime of a newly created or renewed session.
const SessionTTL = 7 * 24 * time.Hour

// Absolute session lifetime ceilings, enforced at renewal time.
// A session whose created_at + ceiling has passed will not be renewed.
const (
	SessionAbsoluteMaxAdmin    = 30 * 24 * time.Hour
	SessionAbsoluteMaxMerchant = 30 * 24 * time.Hour
	SessionAbsoluteMaxBuyer    = 90 * 24 * time.Hour
)

// AbsoluteSessionMax returns the hard lifetime ceiling for the given service.
func AbsoluteSessionMax(service SessionService) time.Duration {
	switch service {
	case SessionServiceAdminPanel:
		return SessionAbsoluteMaxAdmin
	case SessionServiceMerchantPanel:
		return SessionAbsoluteMaxMerchant
	case SessionServiceBuyerPlatform:
		return SessionAbsoluteMaxBuyer
	default:
		return SessionAbsoluteMaxBuyer
	}
}

type OrganizationType string

const (
	OrganizationTypePlatform   OrganizationType = "platform"
	OrganizationTypeMerchant   OrganizationType = "merchant"
	OrganizationTypeIndividual OrganizationType = "individual"
	OrganizationTypeCompany    OrganizationType = "company"
)

type OrganizationCapability string

const (
	OrganizationCapabilityPlatform OrganizationCapability = "platform"
	OrganizationCapabilityBuyer    OrganizationCapability = "buyer"
	OrganizationCapabilitySeller   OrganizationCapability = "seller"
)

type OrganizationStatus string

const (
	OrganizationStatusPending   OrganizationStatus = "pending"
	OrganizationStatusActive    OrganizationStatus = "active"
	OrganizationStatusSuspended OrganizationStatus = "suspended"
)

type ProviderID string

const (
	ProviderIDCredential ProviderID = "credential"
	ProviderIDGoogle     ProviderID = "google"
)

type AddressType string

const (
	AddressShipping  AddressType = "shipping"
	AddressBilling   AddressType = "billing"
	AddressWarehouse AddressType = "warehouse"
	AddressGeneral   AddressType = "general"
)

type ProductAssetType string

const (
	ProductAssetImage    ProductAssetType = "image"
	ProductAssetVideo    ProductAssetType = "video"
	ProductAssetDocument ProductAssetType = "document"
)

type ProductStatus string

const (
	ProductStatusDraft     ProductStatus = "draft"
	ProductStatusActive    ProductStatus = "active"
	ProductStatusArchived  ProductStatus = "archived"
	ProductStatusSuspended ProductStatus = "suspended"
)

type CheckoutSessionStatus string

const (
	CheckoutSessionStatusPending        CheckoutSessionStatus = "pending"
	CheckoutSessionStatusReserved       CheckoutSessionStatus = "reserved"
	CheckoutSessionStatusPaymentPending CheckoutSessionStatus = "payment_pending"
	CheckoutSessionStatusCompleted      CheckoutSessionStatus = "completed"
	CheckoutSessionStatusCancelled      CheckoutSessionStatus = "cancelled"
	CheckoutSessionStatusExpired        CheckoutSessionStatus = "expired"
	CheckoutSessionStatusFailed         CheckoutSessionStatus = "failed"
)

type OrderStatus string

const (
	OrderStatusPendingPayment OrderStatus = "pending_payment"
	OrderStatusPlaced         OrderStatus = "placed"
	OrderStatusProcessing     OrderStatus = "processing"
	OrderStatusCancelled      OrderStatus = "cancelled"
	OrderStatusExpired        OrderStatus = "expired"
	OrderStatusCompleted      OrderStatus = "completed"
)

type OrderPaymentStatus string

const (
	OrderPaymentStatusUnpaid            OrderPaymentStatus = "unpaid"
	OrderPaymentStatusAuthorized        OrderPaymentStatus = "authorized"
	OrderPaymentStatusPaid              OrderPaymentStatus = "paid"
	OrderPaymentStatusFailed            OrderPaymentStatus = "failed"
	OrderPaymentStatusPartiallyRefunded OrderPaymentStatus = "partially_refunded"
	OrderPaymentStatusRefunded          OrderPaymentStatus = "refunded"
)

type OrderFulfillmentStatus string

const (
	OrderFulfillmentStatusUnfulfilled OrderFulfillmentStatus = "unfulfilled"
	OrderFulfillmentStatusProcessing  OrderFulfillmentStatus = "processing"
	OrderFulfillmentStatusShipped     OrderFulfillmentStatus = "shipped"
	OrderFulfillmentStatusDelivered   OrderFulfillmentStatus = "delivered"
	OrderFulfillmentStatusReturned    OrderFulfillmentStatus = "returned"
)

type InventoryReservationStatus string

const (
	InventoryReservationStatusActive    InventoryReservationStatus = "active"
	InventoryReservationStatusConfirmed InventoryReservationStatus = "confirmed"
	InventoryReservationStatusReleased  InventoryReservationStatus = "released"
	InventoryReservationStatusExpired   InventoryReservationStatus = "expired"
	InventoryReservationStatusCancelled InventoryReservationStatus = "cancelled"
)

type OrderStatusHistoryActorType string

const (
	OrderStatusHistoryActorTypeCustomer        OrderStatusHistoryActorType = "customer"
	OrderStatusHistoryActorTypeMerchantMember  OrderStatusHistoryActorType = "merchant_member"
	OrderStatusHistoryActorTypePlatformMember  OrderStatusHistoryActorType = "platform_member"
	OrderStatusHistoryActorTypeSystem          OrderStatusHistoryActorType = "system"
	OrderStatusHistoryActorTypePaymentProvider OrderStatusHistoryActorType = "payment_provider"
)

type PaymentProvider string

const (
	PaymentProviderManual PaymentProvider = "manual"
)

type PaymentStatus string

const (
	PaymentStatusPending           PaymentStatus = "pending"
	PaymentStatusRequiresAction    PaymentStatus = "requires_action"
	PaymentStatusAuthorized        PaymentStatus = "authorized"
	PaymentStatusSucceeded         PaymentStatus = "succeeded"
	PaymentStatusFailed            PaymentStatus = "failed"
	PaymentStatusCancelled         PaymentStatus = "cancelled"
	PaymentStatusPartiallyRefunded PaymentStatus = "partially_refunded"
	PaymentStatusRefunded          PaymentStatus = "refunded"
)

type PaymentTransactionType string

const (
	PaymentTransactionTypeAuthorize PaymentTransactionType = "authorize"
	PaymentTransactionTypeCapture   PaymentTransactionType = "capture"
	PaymentTransactionTypeSale      PaymentTransactionType = "sale"
	PaymentTransactionTypeRefund    PaymentTransactionType = "refund"
	PaymentTransactionTypeVoid      PaymentTransactionType = "void"
)

type PaymentTransactionStatus string

const (
	PaymentTransactionStatusPending   PaymentTransactionStatus = "pending"
	PaymentTransactionStatusSucceeded PaymentTransactionStatus = "succeeded"
	PaymentTransactionStatusFailed    PaymentTransactionStatus = "failed"
	PaymentTransactionStatusCancelled PaymentTransactionStatus = "cancelled"
)
