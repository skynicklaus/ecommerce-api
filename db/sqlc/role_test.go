package db_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/require"
	"github.com/google/uuid"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomRoleWithOrg(t *testing.T, organization *db.Organization) db.Role {
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

	role, err := testStore.CreateRole(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, role)

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

	role2, err := testStore.GetRoleByID(context.Background(), role1.ID)
	require.NoError(t, err)
	require.NotEmpty(t, role2)

	require.Equal(t, role1.ID, role2.ID)
	require.Equal(t, role1.OrganizationID, role2.OrganizationID)
	require.Equal(t, role1.OrganizationType, role2.OrganizationType)
	require.Equal(t, role1.RoleName, role2.RoleName)
	require.Equal(t, role1.Slug, role2.Slug)
	require.Equal(t, role1.IsSystem, role2.IsSystem)
	require.WithinDuration(t, role1.CreatedAt, role2.CreatedAt, time.Second)
	require.WithinDuration(t, role1.UpdatedAt, role2.UpdatedAt, time.Second)
}

func TestGetRoleBySlug(t *testing.T) {
	role1 := createRandomRole(t)

	role2, err := testStore.GetRoleBySlug(context.Background(), role1.Slug)
	require.NoError(t, err)
	require.NotEmpty(t, role2)

	require.Equal(t, role1.ID, role2.ID)
	require.Equal(t, role1.OrganizationID, role2.OrganizationID)
	require.Equal(t, role1.OrganizationType, role2.OrganizationType)
	require.Equal(t, role1.RoleName, role2.RoleName)
	require.Equal(t, role1.Slug, role2.Slug)
	require.Equal(t, role1.IsSystem, role2.IsSystem)
	require.WithinDuration(t, role1.CreatedAt, role2.CreatedAt, time.Second)
	require.WithinDuration(t, role1.UpdatedAt, role2.UpdatedAt, time.Second)
}

// TODO: Test ListOrganizationRolesByType
