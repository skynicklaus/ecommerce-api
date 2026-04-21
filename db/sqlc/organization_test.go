package db_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/go-openapi/testify/v2/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createRandomOrganization(t *testing.T) db.Organization {
	organizationType := util.GetRandomOrganizationType(t)
	require.NotEmpty(t, organizationType)

	slug := fmt.Sprintf("%s.%s", organizationType, util.GetRandomString(t, 8))

	n, err := rand.Int(rand.Reader, big.NewInt(2))
	require.NoError(t, err)

	var metadata []byte
	if n.Int64() == 1 {
		metadata, err = json.Marshal(struct {
			Description string `json:"description"`
		}{
			Description: util.GetRandomString(t, 10),
		})
		require.NoError(t, err)
	}

	organizationStatus := util.GetRandomOrganizationStatus(t)
	require.NotEmpty(t, organizationStatus)

	//nolint:exhaustruct // parent oragnization
	arg := db.CreateOrganizationParams{
		Name:     util.GetRandomString(t, 8),
		Type:     organizationType,
		Slug:     slug,
		Metadata: metadata,
		Status:   organizationStatus,
	}

	organization, err := testStore.CreateOrganization(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, organization)

	require.Equal(t, arg.Name, organization.Name)
	require.Equal(t, arg.Type, organization.Type)
	require.Equal(t, arg.Slug, organization.Slug)
	require.Equal(t, arg.Status, organization.Status)
	require.NotZero(t, organization.CreatedAt)

	if len(metadata) > 0 {
		require.JSONEq(t, string(arg.Metadata), string(organization.Metadata))
	} else {
		require.Empty(t, organization.Metadata)
	}

	return organization
}

func TestCreateOrganization(t *testing.T) {
	createRandomOrganization(t)
}

func TestCreateSubOrganization(t *testing.T) {
	parentOrganization := createRandomOrganization(t)

	organizationType := util.GetRandomOrganizationType(t)
	require.NotEmpty(t, organizationType)

	slug := fmt.Sprintf("%s.%s", organizationType, util.GetRandomString(t, 8))

	n, err := rand.Int(rand.Reader, big.NewInt(2))
	require.NoError(t, err)

	var metadata []byte
	if n.Int64() == 1 {
		metadata, err = json.Marshal(struct {
			Description string `json:"description"`
		}{
			Description: util.GetRandomString(t, 10),
		})
		require.NoError(t, err)
	}

	organizationStatus := util.GetRandomOrganizationStatus(t)
	require.NotEmpty(t, organizationStatus)

	arg := db.CreateOrganizationParams{
		ParentID: &parentOrganization.ID,
		Name:     util.GetRandomString(t, 8),
		Type:     organizationType,
		Slug:     slug,
		Metadata: metadata,
		Status:   organizationStatus,
	}

	chilldOrganization, err := testStore.CreateOrganization(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, chilldOrganization)

	require.NotEmpty(t, chilldOrganization.ParentID)
	require.Equal(t, parentOrganization.ID, *(chilldOrganization.ParentID))
	require.Equal(t, arg.Name, chilldOrganization.Name)
	require.Equal(t, arg.Type, chilldOrganization.Type)
	require.Equal(t, arg.Slug, chilldOrganization.Slug)
	require.Equal(t, arg.Status, chilldOrganization.Status)
	require.NotZero(t, chilldOrganization.CreatedAt)

	if len(metadata) > 0 {
		require.JSONEq(t, string(arg.Metadata), string(chilldOrganization.Metadata))
	} else {
		require.Empty(t, chilldOrganization.Metadata)
	}
}

func TestGetOrganizationByID(t *testing.T) {
	organization1 := createRandomOrganization(t)

	organization2, err := testStore.GetOrganizationByID(context.Background(), organization1.ID)
	require.NoError(t, err)
	require.NotEmpty(t, organization2)

	require.Equal(t, organization1.ID, organization2.ID)
	require.Equal(t, organization1.ParentID, organization2.ParentID)
	require.Equal(t, organization1.Name, organization2.Name)
	require.Equal(t, organization1.Slug, organization2.Slug)
	require.Equal(t, organization1.Type, organization2.Type)
	require.Equal(t, organization1.Logo, organization2.Logo)
	require.Equal(t, organization1.Status, organization2.Status)
	require.WithinDuration(t, organization1.CreatedAt, organization2.CreatedAt, time.Second)
}

func TestGetOrganizationBySlug(t *testing.T) {
	organization1 := createRandomOrganization(t)

	organization2, err := testStore.GetOrganizationBySlug(context.Background(), organization1.Slug)
	require.NoError(t, err)
	require.NotEmpty(t, organization2)

	require.Equal(t, organization1.ID, organization2.ID)
	require.Equal(t, organization1.ParentID, organization2.ParentID)
	require.Equal(t, organization1.Name, organization2.Name)
	require.Equal(t, organization1.Slug, organization2.Slug)
	require.Equal(t, organization1.Type, organization2.Type)
	require.Equal(t, organization1.Logo, organization2.Logo)
	require.Equal(t, organization1.Status, organization2.Status)
	require.WithinDuration(t, organization1.CreatedAt, organization2.CreatedAt, time.Second)
}
