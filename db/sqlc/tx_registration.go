package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/skynicklaus/ecommerce-api/util"
)

type UserInfo struct {
	Name  string
	Email string
}

type AccountInfoParams struct {
	AccountID             string
	ProviderID            string
	AccessToken           *string
	RefreshToken          *string
	AccessTokenExpiresAt  *time.Time
	RefreshTokenExpiresAt *time.Time
	Scope                 *string
	IDToken               *string
	HashedPassword        *string
}

type RegistrationTxParams struct {
	CreateOrganizationParams

	AccountInfoParams    AccountInfoParams
	IdentityType         string
	UserInfo             UserInfo
	RoleID               int16
	RoleOrganizationType string
	RoleAssignBy         *uuid.UUID
}

type RegisteredUser struct {
	ID            uuid.UUID `json:"id"`
	IdentityID    uuid.UUID `json:"identity_id"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	Image         string    `json:"image"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type AccountInfo struct {
	ID                    uuid.UUID
	UserID                uuid.UUID
	AccountID             string
	ProviderID            string
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
	Scope                 string
	IDToken               string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type RegistrationTxResults struct {
	Identity     Identity
	User         RegisteredUser
	AccountInfo  AccountInfo
	Organization Organization
	Member       Member
}

var (
	ErrMismatchOrganizationType     = errors.New("organization type mistmatch")
	ErrMismatchRoleOrganizationType = errors.New("role organization type mistmatch")
	ErrInvalidUserType              = errors.New("invalid user type")
)

func (store *SQLStore) RegistrationTx(
	ctx context.Context,
	arg RegistrationTxParams,
) (RegistrationTxResults, error) {
	var results RegistrationTxResults
	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		if arg.RoleOrganizationType != arg.Type {
			return ErrMismatchRoleOrganizationType
		}

		results.Identity, err = q.CreateIdentity(ctx, arg.IdentityType)
		if err != nil {
			return err
		}

		switch results.Identity.Type {
		case string(util.IdentityUser):
			if arg.Type != string(util.OrganizationTypePlatform) &&
				arg.Type != string(util.OrganizationTypeMerchant) {
				return ErrMismatchOrganizationType
			}

			results.User, results.AccountInfo, err = createUserAndAccount(
				ctx,
				q,
				results.Identity.ID,
				arg,
			)
			if err != nil {
				return err
			}
		case string(util.IdentityCustomer):
			if arg.Type != string(util.OrganizationTypeIndividual) &&
				arg.Type != string(util.OrganizationTypeCompany) {
				return ErrMismatchOrganizationType
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
		default:
			return ErrInvalidUserType
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

func createUserAndAccount(
	ctx context.Context,
	q *Queries,
	identityID uuid.UUID,
	arg RegistrationTxParams,
) (RegisteredUser, AccountInfo, error) {
	user, err := q.CreateUser(ctx, CreateUserParams{
		IdentityID: identityID,
		Name:       arg.UserInfo.Name,
		Email:      arg.UserInfo.Email,
	})
	if err != nil {
		return RegisteredUser{}, AccountInfo{}, err
	}

	regUser := mapUserToRegisteredUser(user)

	userAccount, err := q.CreateUserAccount(ctx, buildAccountParams(arg.AccountInfoParams, user.ID))
	if err != nil {
		return RegisteredUser{}, AccountInfo{}, err
	}

	accountInfo := mapUserAccountToAccountInfo(userAccount)

	return regUser, accountInfo, nil
}

func createCustomerAndAccount(
	ctx context.Context,
	q *Queries,
	identityID uuid.UUID,
	arg RegistrationTxParams,
) (RegisteredUser, AccountInfo, error) {
	customer, err := q.CreateCustomer(ctx, CreateCustomerParams{
		IdentityID: identityID,
		Name:       arg.UserInfo.Name,
		Email:      arg.UserInfo.Email,
	})
	if err != nil {
		return RegisteredUser{}, AccountInfo{}, err
	}

	regUser := mapCustomerToRegisteredUser(customer)

	customerAccount, err := q.CreateCustomerAccount(
		ctx,
		buildCustomerAccountParams(arg.AccountInfoParams, customer.ID),
	)
	if err != nil {
		return RegisteredUser{}, AccountInfo{}, err
	}

	accountInfo := mapCustomerAccountToAccountInfo(customerAccount)

	return regUser, accountInfo, nil
}

//nolint:dupl // because users and customers structures are similar
func buildAccountParams(info AccountInfoParams, userID uuid.UUID) CreateUserAccountParams {
	accountID := info.AccountID
	if info.ProviderID == string(util.ProviderIDCredential) {
		accountID = userID.String()
	}

	return CreateUserAccountParams{
		UserID:                userID,
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

//nolint:dupl // because users and customers structures are similar
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

func mapUserToRegisteredUser(user User) RegisteredUser {
	return RegisteredUser{
		ID:            user.ID,
		IdentityID:    user.IdentityID,
		Name:          user.Name,
		Email:         user.Email,
		EmailVerified: user.EmailVerified,
		Image:         util.DeferString(user.Image),
		CreatedAt:     user.CreatedAt,
		UpdatedAt:     user.UpdatedAt,
	}
}

func mapCustomerToRegisteredUser(customer Customer) RegisteredUser {
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

func mapUserAccountToAccountInfo(userAccount CreateUserAccountRow) AccountInfo {
	return AccountInfo{
		ID:                    userAccount.ID,
		UserID:                userAccount.UserID,
		AccountID:             userAccount.AccountID,
		ProviderID:            userAccount.ProviderID,
		AccessToken:           util.DeferString(userAccount.AccessToken),
		RefreshToken:          util.DeferString(userAccount.RefreshToken),
		AccessTokenExpiresAt:  util.DeferTime(userAccount.AccessTokenExpiresAt),
		RefreshTokenExpiresAt: util.DeferTime(userAccount.RefreshTokenExpiresAt),
		Scope:                 util.DeferString(userAccount.Scope),
		IDToken:               util.DeferString(userAccount.IDToken),
		CreatedAt:             userAccount.CreatedAt,
		UpdatedAt:             userAccount.UpdatedAt,
	}
}

func mapCustomerAccountToAccountInfo(customerAccount CreateCustomerAccountRow) AccountInfo {
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
