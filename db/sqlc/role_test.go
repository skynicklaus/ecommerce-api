package db_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"

	"github.com/go-openapi/testify/v2/require"
	"github.com/google/uuid"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomRole(t *testing.T) db.Role {
	n, err := rand.Int(rand.Reader, big.NewInt(2))
	require.NoError(t, err)
	require.NotEmpty(t, n)

	organizationType := util.GetRandomOrganizationType(t)
	require.NotEmpty(t, organizationType)

	isSystem := true
	slug := util.GetRandomString(t, 8)
	var organizationID *uuid.UUID
	if n.Int64() == 0 {
		isSystem = false
		organization := createRandomOrganization(t)
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
	require.Equal(t, arg.OrganizationType, role.OrganizationType)
	require.Equal(t, arg.Slug, role.Slug)
	require.Equal(t, arg.IsSystem, role.IsSystem)
	require.NotZero(t, role.CreatedAt)

	if !isSystem {
		require.Equal(t, arg.OrganizationID, role.OrganizationID)
	}

	return role
}
