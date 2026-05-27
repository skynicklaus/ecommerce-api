//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomMember(t *testing.T) db.Member {
	t.Helper()
	identity := createRandomIdentity(t)
	organization := createRandomOrganization(t)

	arg := db.CreateMemberParams{
		IdentityID:     identity.ID,
		OrganizationID: organization.ID,
	}

	member, err := testStore.CreateMember(t.Context(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, member)

	require.Equal(t, arg.IdentityID, member.IdentityID)
	require.Equal(t, arg.OrganizationID, member.OrganizationID)

	return member
}

func TestCreateMember(t *testing.T) {
	createRandomMember(t)
}

func TestGetMemberByIdentityID(t *testing.T) {
	member1 := createRandomMember(t)

	member2, err := testStore.GetMemberByIdentityID(t.Context(), member1.IdentityID)
	require.NoError(t, err)
	require.NotEmpty(t, member2)

	require.Equal(t, member1.ID, member2.ID)
	require.Equal(t, member1.IdentityID, member2.IdentityID)
	require.Equal(t, member1.OrganizationID, member2.OrganizationID)
}

func TestCountPlatformAdmins(t *testing.T) {
	initialCount, err := testStore.CountPlatformAdmins(t.Context())
	require.NoError(t, err)

	orgArg := db.CreateOrganizationParams{
		Name:       "Platform Org " + util.GetRandomString(t, 6),
		Slug:       "platform-org-" + util.GetRandomString(t, 6),
		Type:       string(util.OrganizationTypePlatform),
		Capability: string(util.OrganizationCapabilityPlatform),
		Status:     string(util.OrganizationStatusActive),
		Metadata:   []byte(`{}`),
	}
	org, err := testStore.CreateOrganization(t.Context(), orgArg)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
	})

	identity := createRandomIdentity(t)
	_, err = testStore.CreateMember(t.Context(), db.CreateMemberParams{
		IdentityID:     identity.ID,
		OrganizationID: org.ID,
	})
	require.NoError(t, err)

	newCount, err := testStore.CountPlatformAdmins(t.Context())
	require.NoError(t, err)
	require.Equal(t, initialCount+1, newCount)
}
