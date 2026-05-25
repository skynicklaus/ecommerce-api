//go:build integration

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomIdentity(t *testing.T) db.Identity {
	t.Helper()
	identityType := util.GetRandomIdentityType(t)
	require.NotEmpty(t, identityType)

	identity, err := testStore.CreateIdentity(t.Context(), identityType)
	require.NoError(t, err)
	require.NotEmpty(t, identity)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), "DELETE FROM identities WHERE id = $1", identity.ID)
	})

	require.Equal(t, identityType, identity.Type)
	require.NotZero(t, identity.CreatedAt)

	return identity
}

func TestCreateIdentity(t *testing.T) {
	createRandomIdentity(t)
}

func TestGetIdentity(t *testing.T) {
	identity1 := createRandomIdentity(t)

	identity2, err := testStore.GetIdentity(t.Context(), identity1.ID)
	require.NoError(t, err)
	require.NotEmpty(t, identity2)

	require.Equal(t, identity1.ID, identity2.ID)
	require.Equal(t, identity1.Type, identity2.Type)
	require.WithinDuration(t, identity1.CreatedAt, identity2.CreatedAt, 5*time.Second)
}
