//go:build integration

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

func createRandomRoleWithOrg(t *testing.T, organization *db.Organization) db.Role {
	t.Helper()
	organizationType := util.GetRandomOrganizationType(t)
	isSystem := true
	slug := util.GetRandomString(t, 8)
	var organizationID *uuid.UUID
	if organization != nil {
		isSystem = false
		organizationID = &organization.ID
		organizationType = organization.Type
		slug = fmt.Sprintf("%s.%s", organization.Slug, slug)
	}

	arg := db.CreateRoleParams{
		RoleName:         util.GetRandomString(t, 8),
		OrganizationID:   organizationID,
		OrganizationType: organizationType,
		Slug:             slug,
		IsSystem:         isSystem,
	}

	role, err := testStore.CreateRole(t.Context(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, role)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), "DELETE FROM roles WHERE id = $1", role.ID)
	})

	require.Equal(t, arg.RoleName, role.RoleName)
	require.Equal(t, arg.OrganizationID, role.OrganizationID)
	require.Equal(t, arg.OrganizationType, role.OrganizationType)
	require.Equal(t, arg.Slug, role.Slug)
	require.Equal(t, arg.IsSystem, role.IsSystem)
	require.NotZero(t, role.CreatedAt)
	require.NotZero(t, role.UpdatedAt)

	return role
}

func createRandomRole(t *testing.T) db.Role {
	t.Helper()
	n := util.CoinFlip(t)

	if n == 1 {
		org := createRandomOrganization(t)
		return createRandomRoleWithOrg(t, &org)
	}

	return createRandomRoleWithOrg(t, nil)
}

func TestCreateRole(t *testing.T) {
	createRandomRole(t)
}

func TestGetRoleByID(t *testing.T) {
	role1 := createRandomRole(t)

	role2, err := testStore.GetRoleByID(t.Context(), role1.ID)
	require.NoError(t, err)
	require.NotEmpty(t, role2)

	require.Equal(t, role1.ID, role2.ID)
	require.Equal(t, role1.OrganizationID, role2.OrganizationID)
	require.Equal(t, role1.OrganizationType, role2.OrganizationType)
	require.Equal(t, role1.RoleName, role2.RoleName)
	require.Equal(t, role1.Slug, role2.Slug)
	require.Equal(t, role1.IsSystem, role2.IsSystem)
	require.WithinDuration(t, role1.CreatedAt, role2.CreatedAt, 5*time.Second)
	require.WithinDuration(t, role1.UpdatedAt, role2.UpdatedAt, 5*time.Second)
}

func TestGetRoleBySlug(t *testing.T) {
	role1 := createRandomRole(t)

	role2, err := testStore.GetRoleBySlug(t.Context(), role1.Slug)
	require.NoError(t, err)
	require.NotEmpty(t, role2)

	require.Equal(t, role1.ID, role2.ID)
	require.Equal(t, role1.OrganizationID, role2.OrganizationID)
	require.Equal(t, role1.OrganizationType, role2.OrganizationType)
	require.Equal(t, role1.RoleName, role2.RoleName)
	require.Equal(t, role1.Slug, role2.Slug)
	require.Equal(t, role1.IsSystem, role2.IsSystem)
	require.WithinDuration(t, role1.CreatedAt, role2.CreatedAt, 5*time.Second)
	require.WithinDuration(t, role1.UpdatedAt, role2.UpdatedAt, 5*time.Second)
}

func TestListOrganizationRolesByType(t *testing.T) {
	ctx := t.Context()

	makeOrg := func() db.Organization {
		org, err := testStore.CreateOrganization(ctx, db.CreateOrganizationParams{
			ParentID:   nil,
			Name:       util.GetRandomString(t, 8),
			Type:       string(util.OrganizationTypeMerchant),
			Capability: string(util.OrganizationCapabilitySeller),
			Slug:       fmt.Sprintf("merchant.%s", util.GetRandomString(t, 10)),
			Status:     string(util.OrganizationStatusActive),
			Metadata:   nil,
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
		})
		return org
	}

	orgA := makeOrg()
	orgB := makeOrg()

	roleA := createRandomRoleWithOrg(t, &orgA)
	roleB := createRandomRoleWithOrg(t, &orgB)
	sysRole, err := testStore.CreateRole(ctx, db.CreateRoleParams{
		RoleName:         util.GetRandomString(t, 8),
		OrganizationID:   nil,
		OrganizationType: string(util.OrganizationTypeMerchant),
		Slug:             util.GetRandomString(t, 8),
		IsSystem:         true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), "DELETE FROM roles WHERE id = $1", sysRole.ID)
	})

	roles, err := testStore.ListOrganizationRolesByType(ctx, db.ListOrganizationRolesByTypeParams{
		OrganizationID:   &orgA.ID,
		OrganizationType: string(util.OrganizationTypeMerchant),
	})
	require.NoError(t, err)
	require.NotEmpty(t, roles)

	idx := make(map[int16]bool, len(roles))
	for _, r := range roles {
		idx[r.ID] = true
	}

	require.True(t, idx[roleA.ID], "org A role should appear in results")
	require.False(t, idx[roleB.ID], "org B role should not appear in results")
	require.True(t, idx[sysRole.ID], "system role (NULL org_id) should always appear")
}
