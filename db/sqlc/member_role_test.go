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

	var assignedBy *uuid.UUID
	if n.Int64() == 1 {
		oldMember := createRandomMember(t)
		assignedBy = &oldMember.ID
	}

	arg := db.AssignRoleToMemberParams{
		MemberID:   member.ID,
		RoleID:     role.ID,
		AssignedBy: assignedBy,
	}

	err = testStore.AssignRoleToMember(t.Context(), arg)
	require.NoError(t, err)

	// Direct database read-back verification
	var dbMemberID uuid.UUID
	var dbRoleID int16
	var dbAssignedBy *uuid.UUID
	err = testPool.QueryRow(
		t.Context(),
		"SELECT member_id, role_id, assigned_by FROM member_roles WHERE member_id = $1 AND role_id = $2",
		member.ID,
		role.ID,
	).Scan(&dbMemberID, &dbRoleID, &dbAssignedBy)
	require.NoError(t, err)
	require.Equal(t, member.ID, dbMemberID)
	require.Equal(t, role.ID, dbRoleID)
	if assignedBy != nil {
		require.NotNil(t, dbAssignedBy)
		require.Equal(t, *assignedBy, *dbAssignedBy)
	} else {
		require.Nil(t, dbAssignedBy)
	}
}

func TestAssignRoleToMember(t *testing.T) {
	randomAssignRoleToMember(t)
}
