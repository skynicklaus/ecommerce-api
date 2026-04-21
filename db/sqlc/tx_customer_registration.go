package db

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/skynicklaus/ecommerce-api/util"
)

const CustomerIdentity = string(util.IdentityCustomer)

var customerOrganizationTypes = []string{
	string(util.OrganizationTypeIndividual),
	string(util.OrganizationTypeCompany),
}

type CustomerRegistrationTxParams struct {
	CreateOrganizationParams

	AccountInfoParams    AccountInfoParams
	UserInfo             UserInfo
	RoleID               int16
	RoleOrganizationType string
	RoleAssignBy         *uuid.UUID
}

type CustomerRegistrationTxResults struct {
	Identity     Identity
	User         RegisteredUser
	AccountInfo  AccountInfo
	Organization Organization
	Member       Member
}

func (store *SQLStore) CustomerRegistrationTx(
	ctx context.Context,
	arg CustomerRegistrationTxParams,
) (CustomerRegistrationTxResults, error) {
	var results CustomerRegistrationTxResults
	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		if !slices.Contains(customerOrganizationTypes, arg.RoleOrganizationType) ||
			!slices.Contains(customerOrganizationTypes, arg.Type) ||
			arg.RoleOrganizationType != arg.Type {
			return ErrMismatchOrganizationType
		}

		results.Identity, err = q.CreateIdentity(ctx, CustomerIdentity)
		if err != nil {
			return err
		}

		results.User, results.AccountInfo, err = createCustomerAndAccount(
			ctx,
			q,
			results.Identity.ID,
			arg,
		)
		if err != nil {
			return err
		}

		results.Organization, err = q.CreateOrganization(ctx, arg.CreateOrganizationParams)
		if err != nil {
			return err
		}

		results.Member, err = q.CreateMember(ctx, CreateMemberParams{
			IdentityID:     results.Identity.ID,
			OrganizationID: results.Organization.ID,
		})
		if err != nil {
			return err
		}

		err = q.AssignRoleToMember(ctx, AssignRoleToMemberParams{
			MemberID:   results.Member.ID,
			RoleID:     arg.RoleID,
			AssignedBy: arg.RoleAssignBy,
		})
		if err != nil {
			return err
		}

		// TODO: Cart System

		return nil
	})

	return results, err
}

func createCustomerAndAccount(
	ctx context.Context,
	q *Queries,
	identityID uuid.UUID,
	arg CustomerRegistrationTxParams,
) (RegisteredUser, AccountInfo, error) {
	customer, err := q.CreateCustomer(ctx, CreateCustomerParams{
		IdentityID: identityID,
		Name:       arg.UserInfo.Name,
		Email:      arg.UserInfo.Email,
	})
	if err != nil {
		return RegisteredUser{}, AccountInfo{}, err
	}

	regUser := MapCustomerToRegisteredUser(customer)

	customerAccount, err := q.CreateCustomerAccount(
		ctx,
		buildCustomerAccountParams(arg.AccountInfoParams, customer.ID),
	)
	if err != nil {
		return RegisteredUser{}, AccountInfo{}, err
	}

	accountInfo := MapCustomerAccountToAccountInfo(customerAccount)

	return regUser, accountInfo, nil
}

func buildCustomerAccountParams(
	info AccountInfoParams,
	customerID uuid.UUID,
) CreateCustomerAccountParams {
	accountID := info.AccountID
	if info.ProviderID == string(util.ProviderIDCredential) {
		accountID = customerID.String()
	}

	return CreateCustomerAccountParams{
		CustomerID:            customerID,
		AccountID:             accountID,
		ProviderID:            info.ProviderID,
		AccessToken:           info.AccessToken,
		RefreshToken:          info.RefreshToken,
		AccessTokenExpiresAt:  info.AccessTokenExpiresAt,
		RefreshTokenExpiresAt: info.RefreshTokenExpiresAt,
		Scope:                 info.Scope,
		IDToken:               info.IDToken,
		HashedPassword:        info.HashedPassword,
	}
}

func MapCustomerToRegisteredUser(customer Customer) RegisteredUser {
	return RegisteredUser{
		ID:            customer.ID,
		IdentityID:    customer.IdentityID,
		Name:          customer.Name,
		Email:         customer.Email,
		EmailVerified: customer.EmailVerified,
		Image:         util.DeferString(customer.Image),
		CreatedAt:     customer.CreatedAt,
		UpdatedAt:     customer.UpdatedAt,
	}
}

func MapCustomerAccountToAccountInfo(customerAccount CreateCustomerAccountRow) AccountInfo {
	return AccountInfo{
		ID:                    customerAccount.ID,
		UserID:                customerAccount.CustomerID,
		AccountID:             customerAccount.AccountID,
		ProviderID:            customerAccount.ProviderID,
		AccessToken:           util.DeferString(customerAccount.AccessToken),
		RefreshToken:          util.DeferString(customerAccount.RefreshToken),
		AccessTokenExpiresAt:  util.DeferTime(customerAccount.AccessTokenExpiresAt),
		RefreshTokenExpiresAt: util.DeferTime(customerAccount.RefreshTokenExpiresAt),
		Scope:                 util.DeferString(customerAccount.Scope),
		IDToken:               util.DeferString(customerAccount.IDToken),
		CreatedAt:             customerAccount.CreatedAt,
		UpdatedAt:             customerAccount.UpdatedAt,
	}
}
