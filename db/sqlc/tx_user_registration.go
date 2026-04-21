package db

import (
	"context"

	"github.com/google/uuid"

	"github.com/skynicklaus/ecommerce-api/util"
)

const UserIdentity = string(util.IdentityUser)

type PlatformUserRegistrationTxParams struct {
	OrganizationID       uuid.UUID
	AccountInfoParams    AccountInfoParams
	UserInfo             UserInfo
	RoleID               int16
	RoleOrganizationType string
	RoleAssignBy         *uuid.UUID
}

type PlatformUserRegistrationTxResults struct {
	Identity    Identity
	User        RegisteredUser
	AccountInfo AccountInfo
	Member      Member
}

func (store *SQLStore) PlatformUserRegistrationTx(
	ctx context.Context,
	arg PlatformUserRegistrationTxParams,
) (PlatformUserRegistrationTxResults, error) {
	var results PlatformUserRegistrationTxResults
	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		if arg.RoleOrganizationType != string(util.OrganizationTypePlatform) {
			return ErrMismatchOrganizationType
		}

		results.Identity, err = store.CreateIdentity(ctx, UserIdentity)
		if err != nil {
			return err
		}

		results.User, results.AccountInfo, err = createUserAndAccount(
			ctx,
			q,
			createUserAndAccountParams{
				IdentityID:        results.Identity.ID,
				Name:              arg.UserInfo.Name,
				Email:             arg.UserInfo.Email,
				AccountInfoParams: arg.AccountInfoParams,
			},
		)
		if err != nil {
			return err
		}

		results.Member, err = store.CreateMember(ctx, CreateMemberParams{
			OrganizationID: arg.OrganizationID,
			IdentityID:     results.Identity.ID,
		})
		if err != nil {
			return err
		}

		err = store.AssignRoleToMember(ctx, AssignRoleToMemberParams{
			MemberID:   results.Member.ID,
			RoleID:     arg.RoleID,
			AssignedBy: arg.RoleAssignBy,
		})
		if err != nil {
			return err
		}

		return nil
	})
	return results, err
}

type UserRegistrationTxParams struct {
	AccountInfoParams AccountInfoParams
	UserInfo          UserInfo
}

type UserRegistrationTxResults struct {
	Identity    Identity
	User        RegisteredUser
	AccountInfo AccountInfo
}

func (store *SQLStore) UserRegistrationTx(
	ctx context.Context,
	arg UserRegistrationTxParams,
) (UserRegistrationTxResults, error) {
	var results UserRegistrationTxResults

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		results.Identity, err = store.CreateIdentity(ctx, UserIdentity)
		if err != nil {
			return err
		}

		results.User, results.AccountInfo, err = createUserAndAccount(
			ctx,
			q,
			createUserAndAccountParams{
				IdentityID:        results.Identity.ID,
				Name:              arg.UserInfo.Name,
				Email:             arg.UserInfo.Email,
				AccountInfoParams: arg.AccountInfoParams,
			},
		)
		if err != nil {
			return err
		}

		return nil
	})

	return results, err
}

type createUserAndAccountParams struct {
	IdentityID        uuid.UUID
	Name              string
	Email             string
	AccountInfoParams AccountInfoParams
}

func createUserAndAccount(
	ctx context.Context, q *Queries,
	arg createUserAndAccountParams,
) (RegisteredUser, AccountInfo, error) {
	user, err := q.CreateUser(ctx, CreateUserParams{
		IdentityID: arg.IdentityID,
		Name:       arg.Name,
		Email:      arg.Email,
	})
	if err != nil {
		return RegisteredUser{}, AccountInfo{}, err
	}

	regUser := MapUserToRegisteredUser(user)

	userAccount, err := q.CreateUserAccount(ctx, buildAccountParams(arg.AccountInfoParams, user.ID))
	if err != nil {
		return RegisteredUser{}, AccountInfo{}, err
	}

	accountInfo := MapUserAccountToAccountInfo(userAccount)

	return regUser, accountInfo, nil
}

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

func MapUserToRegisteredUser(user User) RegisteredUser {
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

func MapUserAccountToAccountInfo(userAccount CreateUserAccountRow) AccountInfo {
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
