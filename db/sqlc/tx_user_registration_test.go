//nolint:exhaustruct // test file
package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestSQLStore_PlatformUserRegistrationTx(t *testing.T) {
	organization, orgErr := testStore.GetOrganizationBySlug(context.Background(), "platform")
	require.NoError(t, orgErr)
	require.NotEmpty(t, organization)

	tests := []struct {
		name     string
		arg      db.PlatformUserRegistrationTxParams
		wantErr  bool
		matchErr error
		check    func(t *testing.T, got db.PlatformUserRegistrationTxResults, arg db.PlatformUserRegistrationTxParams)
	}{
		{
			name: "credential/success",
			arg: db.PlatformUserRegistrationTxParams{
				RoleID:               1,
				RoleOrganizationType: string(util.OrganizationTypePlatform),
				RoleAssignBy:         getRandomMemberID(t),
				OrganizationID:       organization.ID,
				UserInfo: db.UserInfo{
					Name:  util.GetRandomString(t, 8),
					Email: util.GetRandomEmail(t, 10),
				},
				AccountInfoParams: db.AccountInfoParams{
					ProviderID:     string(util.ProviderIDCredential),
					HashedPassword: util.GetRandomHashedPassword(t, 10),
				},
			},
			wantErr:  false,
			matchErr: nil,
			check: func(t *testing.T, got db.PlatformUserRegistrationTxResults, arg db.PlatformUserRegistrationTxParams) {
				// Identity
				require.Equal(t, string(util.IdentityUser), got.Identity.Type)

				// User
				require.Equal(t, arg.UserInfo.Email, got.User.Email)
				require.Equal(t, arg.UserInfo.Name, got.User.Name)

				// User Account
				require.Equal(t, arg.AccountInfoParams.ProviderID, got.AccountInfo.ProviderID)
				require.NotEmpty(t, got.AccountInfo.AccountID)

				// Member
				require.Equal(t, arg.OrganizationID, got.Member.OrganizationID)
			},
		},
		{
			name: "oauth/success",
			arg: db.PlatformUserRegistrationTxParams{
				RoleID:               1,
				RoleOrganizationType: string(util.OrganizationTypePlatform),
				RoleAssignBy:         getRandomMemberID(t),
				OrganizationID:       organization.ID,
				UserInfo: db.UserInfo{
					Name:  util.GetRandomString(t, 8),
					Email: util.GetRandomEmail(t, 10),
				},
				AccountInfoParams: db.AccountInfoParams{
					ProviderID:            string(util.ProviderIDGoogle),
					AccountID:             util.GetRandomString(t, 10),
					AccessToken:           util.GetRandomStringPtr(t, 10),
					RefreshToken:          util.GetRandomStringPtr(t, 10),
					AccessTokenExpiresAt:  new(time.Now().Add(15 * time.Minute)),
					RefreshTokenExpiresAt: new(time.Now().Add(time.Hour)),
					Scope:                 util.GetRandomStringPtr(t, 10),
					IDToken:               util.GetRandomStringPtr(t, 10),
				},
			},
			wantErr:  false,
			matchErr: nil,
			check: func(t *testing.T, got db.PlatformUserRegistrationTxResults, arg db.PlatformUserRegistrationTxParams) {
				// Identity
				require.Equal(t, string(util.IdentityUser), got.Identity.Type)

				// User
				require.Equal(t, arg.UserInfo.Email, got.User.Email)
				require.Equal(t, arg.UserInfo.Name, got.User.Name)

				// User Account
				require.Equal(t, arg.AccountInfoParams.ProviderID, got.AccountInfo.ProviderID)
				require.Equal(t, arg.AccountInfoParams.AccountID, got.AccountInfo.AccountID)
				require.Equal(t, *arg.AccountInfoParams.AccessToken, got.AccountInfo.AccessToken)
				require.Equal(t, *arg.AccountInfoParams.RefreshToken, got.AccountInfo.RefreshToken)
				require.WithinDuration(
					t,
					*arg.AccountInfoParams.AccessTokenExpiresAt,
					got.AccountInfo.AccessTokenExpiresAt,
					time.Second,
				)
				require.WithinDuration(
					t,
					*arg.AccountInfoParams.RefreshTokenExpiresAt,
					got.AccountInfo.RefreshTokenExpiresAt,
					time.Second,
				)
				require.Equal(t, *arg.AccountInfoParams.Scope, got.AccountInfo.Scope)
				require.Equal(t, *arg.AccountInfoParams.IDToken, got.AccountInfo.IDToken)

				// Member
				require.Equal(t, arg.OrganizationID, got.Member.OrganizationID)
			},
		},
		{
			name: "organization.mismatched/fail",
			arg: db.PlatformUserRegistrationTxParams{
				RoleID:               7,
				RoleOrganizationType: string(util.OrganizationTypeIndividual),
				RoleAssignBy:         getRandomMemberID(t),
				OrganizationID:       organization.ID,
				UserInfo: db.UserInfo{
					Name:  util.GetRandomString(t, 8),
					Email: util.GetRandomEmail(t, 10),
				},
				AccountInfoParams: db.AccountInfoParams{
					ProviderID:     string(util.ProviderIDCredential),
					HashedPassword: util.GetRandomHashedPassword(t, 10),
				},
			},
			wantErr:  true,
			matchErr: db.ErrMismatchOrganizationType,
			check: func(t *testing.T, got db.PlatformUserRegistrationTxResults, _ db.PlatformUserRegistrationTxParams) {
				require.Empty(t, got.Identity)
				require.Empty(t, got.User)
				require.Empty(t, got.AccountInfo)
				require.Empty(t, got.Member)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := testStore.PlatformUserRegistrationTx(context.Background(), tt.arg)

			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, tt.matchErr, err)
			} else {
				require.NoError(t, err)
			}

			if tt.check != nil {
				tt.check(t, got, tt.arg)
			}
		})
	}
}

func TestSQLStore_UserRegistrationTx(t *testing.T) {
	tests := []struct {
		name     string
		arg      db.UserRegistrationTxParams
		wantErr  bool
		matchErr error
		check    func(t *testing.T, got db.UserRegistrationTxResults, arg db.UserRegistrationTxParams)
	}{
		{
			name: "credential/success",
			arg: db.UserRegistrationTxParams{
				UserInfo: db.UserInfo{
					Name:  util.GetRandomString(t, 8),
					Email: util.GetRandomEmail(t, 10),
				},
				AccountInfoParams: db.AccountInfoParams{
					ProviderID:     string(util.ProviderIDCredential),
					HashedPassword: util.GetRandomHashedPassword(t, 10),
				},
			},
			wantErr:  false,
			matchErr: nil,
			check: func(t *testing.T, got db.UserRegistrationTxResults, arg db.UserRegistrationTxParams) {
				// Identity
				require.Equal(t, string(util.IdentityUser), got.Identity.Type)

				// User
				require.Equal(t, arg.UserInfo.Email, got.User.Email)
				require.Equal(t, arg.UserInfo.Name, got.User.Name)

				// User Account
				require.Equal(t, arg.AccountInfoParams.ProviderID, got.AccountInfo.ProviderID)
				require.NotEmpty(t, got.AccountInfo.AccountID)
			},
		},
		{
			name: "oauth/success",
			arg: db.UserRegistrationTxParams{
				UserInfo: db.UserInfo{
					Name:  util.GetRandomString(t, 8),
					Email: util.GetRandomEmail(t, 10),
				},
				AccountInfoParams: db.AccountInfoParams{
					ProviderID:            string(util.ProviderIDGoogle),
					AccountID:             util.GetRandomString(t, 10),
					AccessToken:           util.GetRandomStringPtr(t, 10),
					RefreshToken:          util.GetRandomStringPtr(t, 10),
					AccessTokenExpiresAt:  new(time.Now().Add(15 * time.Minute)),
					RefreshTokenExpiresAt: new(time.Now().Add(time.Hour)),
					Scope:                 util.GetRandomStringPtr(t, 10),
					IDToken:               util.GetRandomStringPtr(t, 10),
				},
			},
			wantErr:  false,
			matchErr: nil,
			check: func(t *testing.T, got db.UserRegistrationTxResults, arg db.UserRegistrationTxParams) {
				// Identity
				require.Equal(t, string(util.IdentityUser), got.Identity.Type)

				// User
				require.Equal(t, arg.UserInfo.Email, got.User.Email)
				require.Equal(t, arg.UserInfo.Name, got.User.Name)

				// User Account
				require.Equal(t, arg.AccountInfoParams.ProviderID, got.AccountInfo.ProviderID)
				require.NotEmpty(t, got.AccountInfo.AccountID)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := testStore.UserRegistrationTx(context.Background(), tt.arg)

			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, tt.matchErr, err)
			} else {
				require.NoError(t, err)
			}

			if tt.check != nil {
				tt.check(t, got, tt.arg)
			}
		})
	}
}
