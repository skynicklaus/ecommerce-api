//go:build integration

//nolint:exhaustruct // test file
package db_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestSQLStore_CustomerRegistrationTx(t *testing.T) {
	ctx := t.Context()

	individualRole, err := testStore.GetRoleBySlug(ctx, "individual.owner")
	require.NoError(t, err)

	tests := []struct {
		name     string
		arg      db.CustomerRegistrationTxParams
		wantErr  bool
		matchErr error
		check    func(t *testing.T, got db.CustomerRegistrationTxResults, arg db.CustomerRegistrationTxParams)
	}{
		{
			name: "credential/success",
			arg: db.CustomerRegistrationTxParams{
				RoleID:               individualRole.ID,
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
					Status:     util.GetRandomOrganizationStatus(t),
					Type:       string(util.OrganizationTypeIndividual),
					Capability: string(util.OrganizationCapabilityBuyer),
					Metadata:   util.GetRandomDescriptionJSON(t, 10),
				},
			},
			wantErr:  false,
			matchErr: nil,
			check: func(t *testing.T, got db.CustomerRegistrationTxResults, arg db.CustomerRegistrationTxParams) {
				// Identity
				require.Equal(t, db.CustomerIdentity, got.Identity.Type)

				// User
				require.Equal(t, arg.UserInfo.Email, got.User.Email)
				require.Equal(t, arg.UserInfo.Name, got.User.Name)

				// User Account
				require.Equal(t, arg.AccountInfoParams.ProviderID, got.AccountInfo.ProviderID)
				require.NotEmpty(t, got.AccountInfo.AccountID)

				// Verify hashed password in DB
				var dbHashedPassword *string
				err := testPool.QueryRow(
					t.Context(),
					"SELECT hashed_password FROM customer_accounts WHERE customer_id = $1 AND provider_id = $2",
					got.User.ID,
					got.AccountInfo.ProviderID,
				).Scan(&dbHashedPassword)
				require.NoError(t, err)
				require.NotNil(t, dbHashedPassword)
				require.Equal(t, *arg.AccountInfoParams.HashedPassword, *dbHashedPassword)

				// Organization
				require.Equal(t, arg.Name, got.Organization.Name)
				require.Equal(t, arg.Slug, got.Organization.Slug)
				require.Equal(t, arg.Type, got.Organization.Type)
				require.Equal(t, arg.Status, got.Organization.Status)
				require.JSONEq(t, string(arg.Metadata), string(got.Organization.Metadata))
			},
		},
		{
			name: "oauth/success",
			arg: db.CustomerRegistrationTxParams{
				RoleID:               individualRole.ID,
				RoleOrganizationType: string(util.OrganizationTypeIndividual),
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
				CreateOrganizationParams: db.CreateOrganizationParams{
					Name: util.GetRandomString(t, 8),
					Slug: fmt.Sprintf(
						"%s.%s.",
						string(util.OrganizationTypeIndividual),
						util.GetRandomString(t, 8),
					),
					Status:     util.GetRandomOrganizationStatus(t),
					Type:       string(util.OrganizationTypeIndividual),
					Capability: string(util.OrganizationCapabilityBuyer),
					Metadata:   util.GetRandomDescriptionJSON(t, 10),
				},
			},
			wantErr:  false,
			matchErr: nil,
			check: func(t *testing.T, got db.CustomerRegistrationTxResults, arg db.CustomerRegistrationTxParams) {
				// Identity
				require.Equal(t, db.CustomerIdentity, got.Identity.Type)

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

				// Organization
				require.Equal(t, arg.Name, got.Organization.Name)
				require.Equal(t, arg.Slug, got.Organization.Slug)
				require.Equal(t, arg.Type, got.Organization.Type)
				require.Equal(t, arg.Status, got.Organization.Status)
				require.JSONEq(t, string(arg.Metadata), string(got.Organization.Metadata))
			},
		},
		{
			name: "organization.mismatched/fail",
			arg: db.CustomerRegistrationTxParams{
				RoleOrganizationType: string(util.OrganizationTypeIndividual),
				CreateOrganizationParams: db.CreateOrganizationParams{
					Type:       string(util.OrganizationTypeMerchant),
					Capability: string(util.OrganizationCapabilitySeller),
				},
			},
			wantErr:  true,
			matchErr: db.ErrMismatchOrganizationType,
			check: func(t *testing.T, got db.CustomerRegistrationTxResults, _ db.CustomerRegistrationTxParams) {
				require.Empty(t, got.Identity)
				require.Empty(t, got.AccountInfo)
				require.Empty(t, got.User)
				require.Empty(t, got.Organization)
				require.Empty(t, got.Member)
			},
		},
		{
			name: "role.mismatched/fail",
			arg: db.CustomerRegistrationTxParams{
				RoleOrganizationType: string(util.OrganizationTypeMerchant),
				CreateOrganizationParams: db.CreateOrganizationParams{
					Type:       string(util.OrganizationTypeIndividual),
					Capability: string(util.OrganizationCapabilityBuyer),
				},
			},
			wantErr:  true,
			matchErr: db.ErrMismatchOrganizationType,
			check: func(t *testing.T, got db.CustomerRegistrationTxResults, _ db.CustomerRegistrationTxParams) {
				require.Empty(t, got.Identity)
				require.Empty(t, got.AccountInfo)
				require.Empty(t, got.User)
				require.Empty(t, got.Organization)
				require.Empty(t, got.Member)
			},
		},
		{
			name: "role.organization.mismatched/fail",
			arg: db.CustomerRegistrationTxParams{
				RoleOrganizationType: string(util.OrganizationTypeIndividual),
				CreateOrganizationParams: db.CreateOrganizationParams{
					Type:       string(util.OrganizationTypeCompany),
					Capability: string(util.OrganizationCapabilitySeller),
				},
			},
			wantErr:  true,
			matchErr: db.ErrMismatchOrganizationType,
			check: func(t *testing.T, got db.CustomerRegistrationTxResults, _ db.CustomerRegistrationTxParams) {
				require.Empty(t, got.Identity)
				require.Empty(t, got.AccountInfo)
				require.Empty(t, got.User)
				require.Empty(t, got.Organization)
				require.Empty(t, got.Member)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := testStore.CustomerRegistrationTx(t.Context(), tt.arg)

			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, tt.matchErr, err)
			} else {
				require.NoError(t, err)
				t.Cleanup(func() {
					_, _ = testPool.Exec(context.Background(), "DELETE FROM identities WHERE id = $1", got.Identity.ID)
					_, _ = testPool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", got.Organization.ID)
				})
			}

			if tt.check != nil {
				tt.check(t, got, tt.arg)
			}
		})
	}
}

func getRandomMemberID(t *testing.T) *uuid.UUID {
	t.Helper()
	member := createRandomMember(t)

	return &member.ID
}
