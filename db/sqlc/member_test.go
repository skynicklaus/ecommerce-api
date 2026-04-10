package db_test

import (
	"context"
	"testing"

	"github.com/go-openapi/testify/v2/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
)

func createRandomMember(t *testing.T) db.Member {
	identity := createRandomIdentity(t)
	organization := createRandomOrganization(t)

	arg := db.CreateMemberParams{
		IdentityID:     identity.ID,
		OrganizationID: organization.ID,
	}

	member, err := testStore.CreateMember(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, member)

	require.Equal(t, arg.IdentityID, member.IdentityID)
	require.Equal(t, arg.OrganizationID, member.OrganizationID)

	return member
}

func TestCreateMember(t *testing.T) {
	createRandomMember(t)
}
