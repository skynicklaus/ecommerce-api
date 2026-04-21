package db_test

import (
	"context"
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/go-openapi/testify/v2/require"
	"github.com/google/uuid"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
)

func randomAssignRoleToMember(t *testing.T) {
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

	err = testStore.AssignRoleToMember(context.Background(), arg)
	require.NoError(t, err)
}

func TestAssignToleToMember(t *testing.T) {
	randomAssignRoleToMember(t)
}
