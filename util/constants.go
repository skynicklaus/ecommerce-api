package util

type IdentityType string

const (
	IdentityUser     IdentityType = "user"
	IdentityCustomer IdentityType = "customer"
)

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
