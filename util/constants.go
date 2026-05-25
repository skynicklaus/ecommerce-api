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
