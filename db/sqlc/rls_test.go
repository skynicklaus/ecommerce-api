//go:build integration

package db_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
)

func TestRLSContextAppliesToDirectQueries(t *testing.T) {
	orgA := createRandomOrganization(t)
	cleanupOrganization(t, orgA.ID.String())
	orgB := createRandomOrganization(t)
	cleanupOrganization(t, orgB.ID.String())
	warehouseB := createRandomWarehouseWithOrg(t, orgB)

	ctx := db.WithRLSContext(t.Context(), db.RLSContext{
		IdentityID:     uuid.New(),
		OrganizationID: orgA.ID,
	})

	rows, err := testStore.ListWarehousesByOrganization(ctx, orgB.ID)
	require.NoError(t, err)
	require.Empty(t, rows)

	_, err = testStore.GetWarehouseByIDAndOrganization(ctx, db.GetWarehouseByIDAndOrganizationParams{
		ID:             warehouseB.ID,
		OrganizationID: orgB.ID,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, pgx.ErrNoRows))
}
