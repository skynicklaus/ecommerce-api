//go:build integration

package db_test

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
)

func randomAssignRoleToMember(t *testing.T) {
	t.Helper()
	member := createRandomMember(t)
	role := createRandomRole(t)

	n, err := rand.Int(rand.Reader, big.NewInt(2))
	require.NoError(t, err)
	require.NotEmpty(t, n)

	var uuid *uuid.UUID
	if n.Int64() == 1 {
		oldMember := createRandomMember(t)
		uuid = &oldMember.ID
	}

	arg := db.AssignRoleToMemberParams{
		MemberID:   member.ID,
		RoleID:     role.ID,
		AssignedBy: uuid,
	}

	err = testStore.AssignRoleToMember(t.Context(), arg)
	require.NoError(t, err)
}

func TestAssignRoleToMember(t *testing.T) {
	randomAssignRoleToMember(t)
}
