//go:build integration

package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
	"github.com/skynicklaus/ecommerce-api/internal/storage"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestManagementHandlers_Integration(t *testing.T) {
	ctx := t.Context()

	dbSource := os.Getenv("DB_SOURCE")
	if dbSource == "" {
		dbSource = "postgresql://app_system:system_secret@localhost:5432/ecommerce?sslmode=disable"
	}

	connPool, err := pgxpool.New(ctx, dbSource)
	require.NoError(t, err)
	defer connPool.Close()

	store := db.NewStore(connPool)
	logger := util.NewLogger()
	redisClient := cache.NewRedis(store, logger)
	defer redisClient.Close()
	s3Storage, err := storage.New(ctx)
	require.NoError(t, err)

	h := NewV1Handler(store, logger, redisClient, s3Storage).(*V1Handler)

	r := chi.NewRouter()
	r.Post("/v1/users/merchant", makeTestHandler(h.UserCredentialRegistration))
	r.Post("/v1/customer", makeTestHandler(h.CustomerCredentialRegistration))
	r.Post("/v1/users/platform", makeTestHandler(h.PlatformUserCredentialRegistration))
	r.Post("/v1/organizations", makeTestHandler(h.CreateOrganization))

	t.Run("UserCredentialRegistration", func(t *testing.T) {
		email := "merchant-reg-" + uuid.New().String()[:8] + "@test.com"

		t.Run("success", func(t *testing.T) {
			body, _ := json.Marshal(UserCredentialRegistrationRequest{
				Name:     "Test Merchant",
				Email:    email,
				Password: "supersecure123",
				RoleSlug: "merchant.owner",
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/users/merchant", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			require.Equal(t, http.StatusCreated, rr.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			require.NotEmpty(t, resp["id"])
		})

		t.Run("duplicate_email_returns_409", func(t *testing.T) {
			body, _ := json.Marshal(UserCredentialRegistrationRequest{
				Name:     "Duplicate",
				Email:    email,
				Password: "supersecure123",
				RoleSlug: "merchant.owner",
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/users/merchant", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			require.Equal(t, http.StatusConflict, rr.Code)
		})

		t.Run("missing_fields_returns_422", func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"name": "No Email"})
			req := httptest.NewRequest(http.MethodPost, "/v1/users/merchant", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
		})
	})

	t.Run("CustomerCredentialRegistration", func(t *testing.T) {
		email := "customer-reg-" + uuid.New().String()[:8] + "@test.com"

		t.Run("success", func(t *testing.T) {
			body, _ := json.Marshal(UserCredentialRegistrationRequest{
				Name:     "Test Buyer",
				Email:    email,
				Password: "supersecure123",
				RoleSlug: "individual.owner",
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/customer", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			require.Equal(t, http.StatusCreated, rr.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			require.NotEmpty(t, resp["id"])
		})

		t.Run("duplicate_credential_returns_409", func(t *testing.T) {
			// Same email: customer already exists, second attempt to add a credential account
			// hits the unique constraint on (customer_id, provider_id).
			body, _ := json.Marshal(UserCredentialRegistrationRequest{
				Name:     "Duplicate Buyer",
				Email:    email,
				Password: "differentpassword123",
				RoleSlug: "individual.owner",
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/customer", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			require.Equal(t, http.StatusConflict, rr.Code)
		})
	})

	t.Run("PlatformUserCredentialRegistration", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			body, _ := json.Marshal(UserCredentialRegistrationRequest{
				Name:     "New Platform Admin",
				Email:    "platform-reg-" + uuid.New().String()[:8] + "@test.com",
				Password: "supersecure123",
				RoleSlug: "platform.owner",
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/users/platform", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			require.Equal(t, http.StatusCreated, rr.Code)

			var resp UseerCredentialRegistrationResults
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			require.NotEmpty(t, resp.User.ID)
		})

		t.Run("invalid_role_returns_400", func(t *testing.T) {
			body, _ := json.Marshal(UserCredentialRegistrationRequest{
				Name:     "Bad Admin",
				Email:    "platform-bad-" + uuid.New().String()[:8] + "@test.com",
				Password: "supersecure123",
				RoleSlug: "nonexistent.role",
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/users/platform", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			require.Equal(t, http.StatusBadRequest, rr.Code)
		})
	})

	t.Run("CreateOrganization", func(t *testing.T) {
		identity, err := store.CreateIdentity(ctx, string(util.IdentityUser))
		require.NoError(t, err)

		t.Run("success", func(t *testing.T) {
			body, _ := json.Marshal(CreateOrganizationRequest{
				IdentityID: identity.ID,
				ParentID:   nil,
				Name:       "Test Merchant Org",
				Slug:       "merchant.test-" + uuid.New().String()[:8],
				Type:       string(util.OrganizationTypeMerchant),
				Status:     string(util.OrganizationStatusPending),
				Metadata:   json.RawMessage(`{}`),
				RoleSlug:   "merchant.owner",
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			require.Equal(t, http.StatusCreated, rr.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			require.NotEmpty(t, resp["id"])
		})

		t.Run("invalid_role_returns_400", func(t *testing.T) {
			body, _ := json.Marshal(CreateOrganizationRequest{
				IdentityID: identity.ID,
				ParentID:   nil,
				Name:       "Bad Role Org",
				Slug:       "merchant.badrole-" + uuid.New().String()[:8],
				Type:       string(util.OrganizationTypeMerchant),
				Status:     string(util.OrganizationStatusPending),
				Metadata:   json.RawMessage(`{}`),
				RoleSlug:   "nonexistent.role",
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			require.Equal(t, http.StatusBadRequest, rr.Code)
		})

		t.Run("missing_fields_returns_422", func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"name": "Incomplete"})
			req := httptest.NewRequest(http.MethodPost, "/v1/organizations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
		})
	})
}
