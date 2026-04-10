//nolint:exhaustruct // test file
package db_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestSQLStore_RegistrationTx(t *testing.T) {
	tests := []struct {
		name     string
		arg      db.RegistrationTxParams
		wantErr  bool
		matchErr error
		check    func(t *testing.T, got db.RegistrationTxResults, arg db.RegistrationTxParams)
	}{
		{
			name: "system.user/success",
			arg: db.RegistrationTxParams{
				IdentityType:         string(util.IdentityUser),
				RoleID:               1,
				RoleOrganizationType: string(util.OrganizationTypePlatform),
				UserInfo: db.UserInfo{
					Name:  util.GetRandomString(t, 10),
					Email: util.GetRandomEmail(t, 10),
				},
				AccountInfoParams: db.AccountInfoParams{
					ProviderID:     "credential",
					HashedPassword: util.GetRandomHashedPassword(t, 10),
				},
				CreateOrganizationParams: db.CreateOrganizationParams{
					Name: util.GetRandomString(t, 8),
					Slug: fmt.Sprintf(
						"%s.%s.",
						util.GetRandomString(t, 8),
						util.GetRandomString(t, 8),
					),
					Status: util.GetRandomOrganizationStatus(t),
					Type:   string(util.OrganizationTypePlatform),
				},
			},
			wantErr: false,
			check: func(t *testing.T, got db.RegistrationTxResults, arg db.RegistrationTxParams) {
				// Identity
				require.Equal(t, arg.IdentityType, got.Identity.Type)

				// User
				require.Equal(t, arg.UserInfo.Email, got.User.Email)
				require.Equal(t, arg.UserInfo.Name, got.User.Name)

				// User Account
				require.Equal(t, arg.AccountInfoParams.ProviderID, got.AccountInfo.ProviderID)

				// Organization
				require.Equal(t, arg.Name, got.Organization.Name)
				require.Equal(t, arg.Slug, got.Organization.Slug)
				require.Equal(t, arg.Type, got.Organization.Type)
				require.Equal(t, arg.Status, got.Organization.Status)
			},
		},
		{
			name: "system.customer/success",
			arg: db.RegistrationTxParams{
				IdentityType:         string(util.IdentityCustomer),
				RoleID:               1,
				RoleOrganizationType: string(util.OrganizationTypeIndividual),
				UserInfo: db.UserInfo{
					Name:  util.GetRandomString(t, 8),
					Email: util.GetRandomEmail(t, 10),
				},
				AccountInfoParams: db.AccountInfoParams{
					ProviderID:     "credential",
					HashedPassword: util.GetRandomHashedPassword(t, 10),
				},
				CreateOrganizationParams: db.CreateOrganizationParams{
					Name: util.GetRandomString(t, 8),
					Slug: fmt.Sprintf(
						"%s.%s.",
						util.GetRandomString(t, 8),
						util.GetRandomString(t, 8),
					),
					Status: util.GetRandomOrganizationStatus(t),
					Type:   string(util.OrganizationTypeIndividual),
				},
			},
			wantErr: false,
			check: func(t *testing.T, got db.RegistrationTxResults, arg db.RegistrationTxParams) {
				// Identity
				require.Equal(t, arg.IdentityType, got.Identity.Type)

				// User
				require.Equal(t, arg.UserInfo.Email, got.User.Email)
				require.Equal(t, arg.UserInfo.Name, got.User.Name)

				// User Account
				require.Equal(t, arg.AccountInfoParams.ProviderID, got.AccountInfo.ProviderID)

				// Organization
				require.Equal(t, arg.Name, got.Organization.Name)
				require.Equal(t, arg.Slug, got.Organization.Slug)
				require.Equal(t, arg.Type, got.Organization.Type)
				require.Equal(t, arg.Status, got.Organization.Status)
			},
		},
		{
			name: "assigned.user/success",
			arg: db.RegistrationTxParams{
				IdentityType:         string(util.IdentityUser),
				RoleID:               1,
				RoleOrganizationType: string(util.OrganizationTypePlatform),
				RoleAssignBy:         getRandomMemberID(t),
				UserInfo: db.UserInfo{
					Name:  util.GetRandomString(t, 10),
					Email: util.GetRandomEmail(t, 10),
				},
				AccountInfoParams: db.AccountInfoParams{
					ProviderID:     "credential",
					HashedPassword: util.GetRandomHashedPassword(t, 10),
				},
				CreateOrganizationParams: db.CreateOrganizationParams{
					Name: util.GetRandomString(t, 8),
					Slug: fmt.Sprintf(
						"%s.%s.",
						util.GetRandomString(t, 8),
						util.GetRandomString(t, 8),
					),
					Status: util.GetRandomOrganizationStatus(t),
					Type:   string(util.OrganizationTypePlatform),
				},
			},
			wantErr: false,
			check: func(t *testing.T, got db.RegistrationTxResults, arg db.RegistrationTxParams) {
				// Identity
				require.Equal(t, arg.IdentityType, got.Identity.Type)

				// User
				require.Equal(t, arg.UserInfo.Email, got.User.Email)
				require.Equal(t, arg.UserInfo.Name, got.User.Name)

				// User Account
				require.Equal(t, arg.AccountInfoParams.ProviderID, got.AccountInfo.ProviderID)

				// Organization
				require.Equal(t, arg.Name, got.Organization.Name)
				require.Equal(t, arg.Slug, got.Organization.Slug)
				require.Equal(t, arg.Type, got.Organization.Type)
				require.Equal(t, arg.Status, got.Organization.Status)
			},
		},
		{
			name: "system.user/fail role organization type mismatch",
			arg: db.RegistrationTxParams{
				IdentityType:         string(util.IdentityUser),
				RoleID:               1,
				RoleOrganizationType: string(util.OrganizationTypePlatform),
				UserInfo: db.UserInfo{
					Name:  util.GetRandomString(t, 10),
					Email: util.GetRandomEmail(t, 10),
				},
				AccountInfoParams: db.AccountInfoParams{
					ProviderID:     "credential",
					HashedPassword: util.GetRandomHashedPassword(t, 10),
				},
				CreateOrganizationParams: db.CreateOrganizationParams{
					Name: util.GetRandomString(t, 8),
					Slug: fmt.Sprintf(
						"%s.%s.",
						util.GetRandomString(t, 8),
						util.GetRandomString(t, 8),
					),
					Status: util.GetRandomOrganizationStatus(t),
					Type:   string(util.OrganizationTypeMerchant),
				},
			},
			wantErr:  true,
			matchErr: db.ErrMismatchRoleOrganizationType,
			check: func(t *testing.T, got db.RegistrationTxResults, _ db.RegistrationTxParams) {
				// Identity
				require.Empty(t, got.Identity)

				// User
				require.Empty(t, got.User)

				// User Account
				require.Empty(t, got.AccountInfo)

				// Organization
				require.Empty(t, got.Organization)
			},
		},
		{
			name: "system.customer/fail role organization type mismatch",
			arg: db.RegistrationTxParams{
				IdentityType:         string(util.IdentityCustomer),
				RoleID:               1,
				RoleOrganizationType: string(util.OrganizationTypeIndividual),
				UserInfo: db.UserInfo{
					Name:  util.GetRandomString(t, 8),
					Email: util.GetRandomEmail(t, 10),
				},
				AccountInfoParams: db.AccountInfoParams{
					ProviderID:     "credential",
					HashedPassword: util.GetRandomHashedPassword(t, 10),
				},
				CreateOrganizationParams: db.CreateOrganizationParams{
					Name: util.GetRandomString(t, 8),
					Slug: fmt.Sprintf(
						"%s.%s.",
						util.GetRandomString(t, 8),
						util.GetRandomString(t, 8),
					),
					Status: util.GetRandomOrganizationStatus(t),
					Type:   string(util.OrganizationTypeMerchant),
				},
			},
			wantErr:  true,
			matchErr: db.ErrMismatchRoleOrganizationType,
			check: func(t *testing.T, got db.RegistrationTxResults, _ db.RegistrationTxParams) {
				// Identity
				require.Empty(t, got.Identity)

				// User
				require.Empty(t, got.User)

				// User Account
				require.Empty(t, got.AccountInfo)

				// Organization
				require.Empty(t, got.Organization)
			},
		},
		{
			name: "system.user/fail organization type mismatch",
			arg: db.RegistrationTxParams{
				IdentityType:         string(util.IdentityUser),
				RoleID:               1,
				RoleOrganizationType: string(util.OrganizationTypeIndividual),
				UserInfo: db.UserInfo{
					Name:  util.GetRandomString(t, 8),
					Email: util.GetRandomEmail(t, 10),
				},
				AccountInfoParams: db.AccountInfoParams{
					ProviderID:     "credential",
					HashedPassword: util.GetRandomHashedPassword(t, 10),
				},
				CreateOrganizationParams: db.CreateOrganizationParams{
					Name: util.GetRandomString(t, 8),
					Slug: fmt.Sprintf(
						"%s.%s.",
						util.GetRandomString(t, 8),
						util.GetRandomString(t, 8),
					),
					Status: util.GetRandomOrganizationStatus(t),
					Type:   string(util.OrganizationTypeIndividual),
				},
			},
			wantErr:  true,
			matchErr: db.ErrMismatchOrganizationType,
			check: func(t *testing.T, got db.RegistrationTxResults, arg db.RegistrationTxParams) {
				// Identity
				require.Equal(t, arg.IdentityType, got.Identity.Type)

				// User
				require.Empty(t, got.User)

				// User Account
				require.Empty(t, got.AccountInfo)

				// Organization
				require.Empty(t, got.Organization)
			},
		},
		{
			name: "system.customer/fail organization type mismatch",
			arg: db.RegistrationTxParams{
				IdentityType:         string(util.IdentityCustomer),
				RoleID:               1,
				RoleOrganizationType: string(util.OrganizationTypePlatform),
				UserInfo: db.UserInfo{
					Name:  util.GetRandomString(t, 8),
					Email: util.GetRandomEmail(t, 10),
				},
				AccountInfoParams: db.AccountInfoParams{
					ProviderID:     "credential",
					HashedPassword: util.GetRandomHashedPassword(t, 10),
				},
				CreateOrganizationParams: db.CreateOrganizationParams{
					Name: util.GetRandomString(t, 8),
					Slug: fmt.Sprintf(
						"%s.%s.",
						util.GetRandomString(t, 8),
						util.GetRandomString(t, 8),
					),
					Status: util.GetRandomOrganizationStatus(t),
					Type:   string(util.OrganizationTypePlatform),
				},
			},
			wantErr:  true,
			matchErr: db.ErrMismatchOrganizationType,
			check: func(t *testing.T, got db.RegistrationTxResults, arg db.RegistrationTxParams) {
				// Identity
				require.Equal(t, arg.IdentityType, got.Identity.Type)

				// User
				require.Empty(t, got.User)

				// User Account
				require.Empty(t, got.AccountInfo)

				// Organization
				require.Empty(t, got.Organization)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := testStore.RegistrationTx(context.Background(), tt.arg)

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

func getRandomMemberID(t *testing.T) *uuid.UUID {
	member := createRandomMember(t)

	return &member.ID
}
