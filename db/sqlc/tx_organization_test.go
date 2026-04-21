package db_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestSQLStore_CreateOrganizationTx(t *testing.T) {
	identity := createRandomIdentity(t)
	organization := createRandomOrganization(t)

	tests := []struct {
		name     string // description of this test case
		arg      db.CreateOrganizationTxRequest
		wantErr  bool
		matchErr error
		check    func(t *testing.T, got db.CreateOrganizationTxResponse, arg db.CreateOrganizationTxRequest)
	}{
		{
			name: "parent/success",
			arg: db.CreateOrganizationTxRequest{
				IdentityID: identity.ID,
				ParentID:   nil,
				Name:       util.GetRandomString(t, 8),
				Slug: fmt.Sprintf(
					"%s.%s",
					string(util.OrganizationTypeMerchant),
					util.GetRandomString(t, 8),
				),
				Type:                 string(util.OrganizationTypeMerchant),
				Metadata:             util.GetRandomDescriptionJSON(t, 10),
				Status:               util.GetRandomOrganizationStatus(t),
				RoleID:               4,
				RoleOrganizationType: string(util.OrganizationTypeMerchant),
			},
			wantErr:  false,
			matchErr: nil,
			check: func(t *testing.T, got db.CreateOrganizationTxResponse, arg db.CreateOrganizationTxRequest) {
				require.Equal(t, arg.IdentityID, got.Member.IdentityID)
				require.Equal(t, arg.ParentID, got.Organization.ParentID)
				require.Equal(t, arg.Name, got.Organization.Name)
				require.Equal(t, arg.Slug, got.Organization.Slug)
				require.Equal(t, arg.Type, got.Organization.Type)
				require.JSONEq(t, string(arg.Metadata), string(got.Organization.Metadata))
				require.Equal(t, arg.Status, got.Organization.Status)
			},
		},
		{
			name: "child/success",
			arg: db.CreateOrganizationTxRequest{
				IdentityID: identity.ID,
				ParentID:   &organization.ID,
				Name:       util.GetRandomString(t, 8),
				Slug: fmt.Sprintf(
					"%s.%s",
					string(util.OrganizationTypeMerchant),
					util.GetRandomString(t, 8),
				),
				Type:                 string(util.OrganizationTypeMerchant),
				Metadata:             util.GetRandomDescriptionJSON(t, 10),
				Status:               util.GetRandomOrganizationStatus(t),
				RoleID:               4,
				RoleOrganizationType: string(util.OrganizationTypeMerchant),
			},
			wantErr:  false,
			matchErr: nil,
			check: func(t *testing.T, got db.CreateOrganizationTxResponse, arg db.CreateOrganizationTxRequest) {
				require.Equal(t, arg.IdentityID, got.Member.IdentityID)
				require.Equal(t, arg.ParentID, got.Organization.ParentID)
				require.Equal(t, arg.Name, got.Organization.Name)
				require.Equal(t, arg.Slug, got.Organization.Slug)
				require.Equal(t, arg.Type, got.Organization.Type)
				require.JSONEq(t, string(arg.Metadata), string(got.Organization.Metadata))
				require.Equal(t, arg.Status, got.Organization.Status)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := testStore.CreateOrganizationTx(context.Background(), tt.arg)

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
