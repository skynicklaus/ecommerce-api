//go:build integration

package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
	"github.com/skynicklaus/ecommerce-api/internal/middleware"
	"github.com/skynicklaus/ecommerce-api/internal/password"
	"github.com/skynicklaus/ecommerce-api/internal/storage"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestAuthFlows_Integration(t *testing.T) {
	ctx := t.Context()

	// 1. Read configuration from environment
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
	midware := middleware.New(store)

	// Create test Chi router
	r := chi.NewRouter()

	// Open Login Routes
	r.Post("/v1/auth/customer/login", makeTestHandler(h.LoginCustomer))
	r.Post("/v1/auth/merchant/login", makeTestHandler(h.LoginMerchant))
	r.Post("/v1/auth/admin/login", makeTestHandler(h.LoginAdmin))

	// Protected Customer Group
	r.Group(func(r chi.Router) {
		r.Use(midware.RequireService(util.SessionServiceBuyerPlatform))
		r.Post("/v1/auth/customer/logout", makeTestHandler(h.Logout))
		r.Get("/v1/auth/customer/me", makeTestHandler(h.GetMe))
		r.Get("/v1/auth/customer/sessions", makeTestHandler(h.ListActiveSessions))
		r.Delete("/v1/auth/customer/sessions", makeTestHandler(h.RevokeOtherSessions))
		r.Delete("/v1/auth/customer/sessions/{id}", makeTestHandler(h.RevokeSessionByID))
	})

	// Protected Merchant Group
	r.Group(func(r chi.Router) {
		r.Use(midware.RequireService(util.SessionServiceMerchantPanel))
		r.Use(midware.ValidateOrganization)
		r.Post("/v1/auth/merchant/logout", makeTestHandler(h.Logout))
		r.Get("/v1/auth/merchant/me", makeTestHandler(h.GetMe))
	})

	// Seed Organization
	org, err := store.CreateOrganization(ctx, db.CreateOrganizationParams{
		Name:     "Auth Test Merchant Org",
		Type:     "merchant",
		Slug:     "auth.merchant-" + uuid.New().String()[:8],
		Status:   "active",
		ParentID: nil,
		Metadata: nil,
	})
	require.NoError(t, err)

	pass := "supersecure123"
	hashedPass, err := password.HashPassword(pass)
	require.NoError(t, err)

	// A. SEED CUSTOMER
	custEmail := "buyer-" + uuid.New().String()[:8] + "@test.com"
	custIdent, err := store.CreateIdentity(ctx, string(util.IdentityCustomer))
	require.NoError(t, err)
	customer, err := store.CreateCustomer(ctx, db.CreateCustomerParams{
		IdentityID: custIdent.ID,
		Name:       "John Buyer",
		Email:      custEmail,
	})
	require.NoError(t, err)
	_, err = store.CreateCustomerAccount(ctx, db.CreateCustomerAccountParams{
		CustomerID:            customer.ID,
		AccountID:             "credential-" + uuid.New().String()[:8],
		ProviderID:            "credential",
		AccessToken:           nil,
		RefreshToken:          nil,
		AccessTokenExpiresAt:  nil,
		RefreshTokenExpiresAt: nil,
		Scope:                 nil,
		IDToken:               nil,
		HashedPassword:        &hashedPass,
	})
	require.NoError(t, err)

	// B. SEED MERCHANT USER
	merchEmail := "merchant-" + uuid.New().String()[:8] + "@test.com"
	merchIdent, err := store.CreateIdentity(ctx, string(util.IdentityUser))
	require.NoError(t, err)
	merchUser, err := store.CreateUser(ctx, db.CreateUserParams{
		IdentityID: merchIdent.ID,
		Name:       "Jane Merchant",
		Email:      merchEmail,
	})
	require.NoError(t, err)
	_, err = store.CreateUserAccount(ctx, db.CreateUserAccountParams{
		UserID:                merchUser.ID,
		AccountID:             "credential-" + uuid.New().String()[:8],
		ProviderID:            "credential",
		AccessToken:           nil,
		RefreshToken:          nil,
		AccessTokenExpiresAt:  nil,
		RefreshTokenExpiresAt: nil,
		Scope:                 nil,
		IDToken:               nil,
		HashedPassword:        &hashedPass,
	})
	require.NoError(t, err)
	_, err = store.CreateMember(ctx, db.CreateMemberParams{
		IdentityID:     merchIdent.ID,
		OrganizationID: org.ID,
	})
	require.NoError(t, err)

	t.Run("Customer Login Success", func(t *testing.T) {
		reqBody, _ := json.Marshal(LoginRequest{
			Email:    custEmail,
			Password: pass,
		})
		req, _ := http.NewRequest("POST", "/v1/auth/customer/login", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		var resp LoginResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Token)
		require.Equal(t, string(util.IdentityCustomer), resp.Identity.Type)
		require.Equal(t, util.SessionServiceBuyerPlatform, resp.Identity.Service)

		// Verify we can access customer me
		meReq, _ := http.NewRequest("GET", "/v1/auth/customer/me", nil)
		meReq.Header.Set("Authorization", "Bearer "+resp.Token)
		meRR := httptest.NewRecorder()
		r.ServeHTTP(meRR, meReq)
		require.Equal(t, http.StatusOK, meRR.Code)

		var meResp middleware.IdentityContext
		err = json.Unmarshal(meRR.Body.Bytes(), &meResp)
		require.NoError(t, err)
		require.Equal(t, string(util.IdentityCustomer), meResp.Type)
		require.Equal(t, custEmail, meResp.Email)

		// Verify customer CANNOT access merchant me (Boundary Check)
		merchReq, _ := http.NewRequest("GET", "/v1/auth/merchant/me", nil)
		merchReq.Header.Set("Authorization", "Bearer "+resp.Token)
		merchRR := httptest.NewRecorder()
		r.ServeHTTP(merchRR, merchReq)
		require.Equal(t, http.StatusForbidden, merchRR.Code)

		// Test Customer Logout
		outReq, _ := http.NewRequest("POST", "/v1/auth/customer/logout", nil)
		outReq.Header.Set("Authorization", "Bearer "+resp.Token)
		outRR := httptest.NewRecorder()
		r.ServeHTTP(outRR, outReq)
		require.Equal(t, http.StatusOK, outRR.Code)

		// Test Customer Me fails post-logout
		meReq2, _ := http.NewRequest("GET", "/v1/auth/customer/me", nil)
		meReq2.Header.Set("Authorization", "Bearer "+resp.Token)
		meRR2 := httptest.NewRecorder()
		r.ServeHTTP(meRR2, meReq2)
		require.Equal(t, http.StatusUnauthorized, meRR2.Code)
	})

	t.Run("Merchant Login Success", func(t *testing.T) {
		reqBody, _ := json.Marshal(LoginRequest{
			Email:    merchEmail,
			Password: pass,
		})
		req, _ := http.NewRequest("POST", "/v1/auth/merchant/login", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		var resp LoginResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Token)
		require.Equal(t, string(util.IdentityUser), resp.Identity.Type)
		require.Equal(t, util.SessionServiceMerchantPanel, resp.Identity.Service)
		require.Equal(t, org.ID, resp.Identity.OrganizationID)

		// Verify merchant me
		meReq, _ := http.NewRequest("GET", "/v1/auth/merchant/me", nil)
		meReq.Header.Set("Authorization", "Bearer "+resp.Token)
		meRR := httptest.NewRecorder()
		r.ServeHTTP(meRR, meReq)
		require.Equal(t, http.StatusOK, meRR.Code)

		var meResp middleware.IdentityContext
		err = json.Unmarshal(meRR.Body.Bytes(), &meResp)
		require.NoError(t, err)
		require.Equal(t, string(util.IdentityUser), meResp.Type)
		require.Equal(t, org.ID, meResp.OrganizationID)

		// Verify merchant CANNOT access customer me (Boundary Check)
		custReq, _ := http.NewRequest("GET", "/v1/auth/customer/me", nil)
		custReq.Header.Set("Authorization", "Bearer "+resp.Token)
		custRR := httptest.NewRecorder()
		r.ServeHTTP(custRR, custReq)
		require.Equal(t, http.StatusForbidden, custRR.Code)
	})

	t.Run("Login Failure Bad Pass", func(t *testing.T) {
		reqBody, _ := json.Marshal(LoginRequest{
			Email:    custEmail,
			Password: "wrongpassword",
		})
		req, _ := http.NewRequest("POST", "/v1/auth/customer/login", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Login Failure Body Too Large", func(t *testing.T) {
		largePassword := make([]byte, 1024*1024+100)
		for i := range largePassword {
			largePassword[i] = 'a'
		}
		reqBody, _ := json.Marshal(LoginRequest{
			Email:    "test@test.com",
			Password: string(largePassword),
		})
		req, _ := http.NewRequest("POST", "/v1/auth/customer/login", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	})

	t.Run("Session Management Lifecycle", func(t *testing.T) {
		reqBody1, _ := json.Marshal(LoginRequest{
			Email:    custEmail,
			Password: pass,
		})
		req1, _ := http.NewRequest("POST", "/v1/auth/customer/login", bytes.NewBuffer(reqBody1))
		req1.Header.Set("Content-Type", "application/json")
		rr1 := httptest.NewRecorder()
		r.ServeHTTP(rr1, req1)
		require.Equal(t, http.StatusOK, rr1.Code)

		var resp1 LoginResponse
		require.NoError(t, json.NewDecoder(rr1.Body).Decode(&resp1))
		token1 := resp1.Token

		req2, _ := http.NewRequest("POST", "/v1/auth/customer/login", bytes.NewBuffer(reqBody1))
		req2.Header.Set("Content-Type", "application/json")
		rr2 := httptest.NewRecorder()
		r.ServeHTTP(rr2, req2)
		require.Equal(t, http.StatusOK, rr2.Code)

		var resp2 LoginResponse
		require.NoError(t, json.NewDecoder(rr2.Body).Decode(&resp2))
		token2 := resp2.Token
		require.NotEmpty(t, token2)
		require.NotEqual(t, token1, token2, "concurrent logins must produce distinct tokens")

		listReq, _ := http.NewRequest("GET", "/v1/auth/customer/sessions", nil)
		listReq.Header.Set("Authorization", "Bearer "+token1)
		listRR := httptest.NewRecorder()
		r.ServeHTTP(listRR, listReq)
		require.Equal(t, http.StatusOK, listRR.Code)

		var sessions []SessionMetadataResponse
		require.NoError(t, json.NewDecoder(listRR.Body).Decode(&sessions))
		require.Len(t, sessions, 2)

		var currentSessionID uuid.UUID
		var otherSessionID uuid.UUID
		for _, s := range sessions {
			if s.IsCurrent {
				currentSessionID = s.ID
			} else {
				otherSessionID = s.ID
			}
		}
		require.NotEqual(t, uuid.Nil, currentSessionID)
		require.NotEqual(t, uuid.Nil, otherSessionID)

		revokeReq, _ := http.NewRequest(
			"DELETE",
			"/v1/auth/customer/sessions/"+otherSessionID.String(),
			nil,
		)
		revokeReq.Header.Set("Authorization", "Bearer "+token1)
		revokeRR := httptest.NewRecorder()
		r.ServeHTTP(revokeRR, revokeReq)
		require.Equal(t, http.StatusOK, revokeRR.Code)

		listRR2 := httptest.NewRecorder()
		r.ServeHTTP(listRR2, listReq)
		require.Equal(t, http.StatusOK, listRR2.Code)
		require.NoError(t, json.NewDecoder(listRR2.Body).Decode(&sessions))
		require.Len(t, sessions, 1)
		require.Equal(t, currentSessionID, sessions[0].ID)

		req3, _ := http.NewRequest("POST", "/v1/auth/customer/login", bytes.NewBuffer(reqBody1))
		req3.Header.Set("Content-Type", "application/json")
		rr3 := httptest.NewRecorder()
		r.ServeHTTP(rr3, req3)
		require.Equal(t, http.StatusOK, rr3.Code)

		revokeOtherReq, _ := http.NewRequest("DELETE", "/v1/auth/customer/sessions", nil)
		revokeOtherReq.Header.Set("Authorization", "Bearer "+token1)
		revokeOtherRR := httptest.NewRecorder()
		r.ServeHTTP(revokeOtherRR, revokeOtherReq)
		require.Equal(t, http.StatusOK, revokeOtherRR.Code)

		listRR3 := httptest.NewRecorder()
		r.ServeHTTP(listRR3, listReq)
		require.Equal(t, http.StatusOK, listRR3.Code)
		require.NoError(t, json.NewDecoder(listRR3.Body).Decode(&sessions))
		require.Len(t, sessions, 1)
		require.True(t, sessions[0].IsCurrent)
	})
}

func TestAdminLoginFlow_Integration(t *testing.T) {
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
	midware := middleware.New(store)

	r := chi.NewRouter()
	r.Post("/v1/auth/admin/login", makeTestHandler(h.LoginAdmin))
	r.Post("/v1/auth/customer/login", makeTestHandler(h.LoginCustomer))
	r.Group(func(r chi.Router) {
		r.Use(midware.RequireService(util.SessionServiceAdminPanel))
		r.Get("/v1/auth/admin/me", makeTestHandler(h.GetMe))
		r.Post("/v1/auth/admin/logout", makeTestHandler(h.Logout))
	})
	r.Group(func(r chi.Router) {
		r.Use(midware.RequireService(util.SessionServiceBuyerPlatform))
		r.Get("/v1/auth/customer/me", makeTestHandler(h.GetMe))
	})

	// Seed admin user — no platform org membership required for login itself.
	adminEmail := "admin-flow-" + uuid.New().String()[:8] + "@test.com"
	pass := "supersecure123"
	hashedPass, err := password.HashPassword(pass)
	require.NoError(t, err)

	adminIdent, err := store.CreateIdentity(ctx, string(util.IdentityUser))
	require.NoError(t, err)
	adminUser, err := store.CreateUser(ctx, db.CreateUserParams{
		IdentityID: adminIdent.ID,
		Name:       "Platform Admin",
		Email:      adminEmail,
	})
	require.NoError(t, err)
	_, err = store.CreateUserAccount(ctx, db.CreateUserAccountParams{
		UserID:                adminUser.ID,
		AccountID:             "credential-" + uuid.New().String()[:8],
		ProviderID:            "credential",
		HashedPassword:        &hashedPass,
		AccessToken:           nil,
		RefreshToken:          nil,
		AccessTokenExpiresAt:  nil,
		RefreshTokenExpiresAt: nil,
		IDToken:               nil,
		Scope:                 nil,
	})
	require.NoError(t, err)

	t.Run("success returns admin_panel session", func(t *testing.T) {
		body, _ := json.Marshal(LoginRequest{Email: adminEmail, Password: pass})
		req, _ := http.NewRequest(http.MethodPost, "/v1/auth/admin/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		var resp LoginResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.NotEmpty(t, resp.Token)
		require.Equal(t, util.SessionServiceAdminPanel, resp.Identity.Service)
		require.Equal(t, string(util.IdentityUser), resp.Identity.Type)
		require.Equal(t, adminEmail, resp.Identity.Email)

		// Token accepted on admin routes.
		meReq, _ := http.NewRequest(http.MethodGet, "/v1/auth/admin/me", nil)
		meReq.Header.Set("Authorization", "Bearer "+resp.Token)
		meRR := httptest.NewRecorder()
		r.ServeHTTP(meRR, meReq)
		require.Equal(t, http.StatusOK, meRR.Code)

		// Token rejected on customer routes (service boundary).
		custReq, _ := http.NewRequest(http.MethodGet, "/v1/auth/customer/me", nil)
		custReq.Header.Set("Authorization", "Bearer "+resp.Token)
		custRR := httptest.NewRecorder()
		r.ServeHTTP(custRR, custReq)
		require.Equal(t, http.StatusForbidden, custRR.Code)

		// Logout invalidates the token.
		outReq, _ := http.NewRequest(http.MethodPost, "/v1/auth/admin/logout", nil)
		outReq.Header.Set("Authorization", "Bearer "+resp.Token)
		outRR := httptest.NewRecorder()
		r.ServeHTTP(outRR, outReq)
		require.Equal(t, http.StatusOK, outRR.Code)

		meReq2, _ := http.NewRequest(http.MethodGet, "/v1/auth/admin/me", nil)
		meReq2.Header.Set("Authorization", "Bearer "+resp.Token)
		meRR2 := httptest.NewRecorder()
		r.ServeHTTP(meRR2, meReq2)
		require.Equal(t, http.StatusUnauthorized, meRR2.Code)
	})

	t.Run("wrong password returns 401", func(t *testing.T) {
		body, _ := json.Marshal(LoginRequest{Email: adminEmail, Password: "wrongpassword"})
		req, _ := http.NewRequest(http.MethodPost, "/v1/auth/admin/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("unknown email returns 401", func(t *testing.T) {
		body, _ := json.Marshal(LoginRequest{Email: "ghost@nowhere.com", Password: "anypassword"})
		req, _ := http.NewRequest(http.MethodPost, "/v1/auth/admin/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func makeTestHandler(h func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			var apiErr apierror.APIError
			if errors.As(err, &apiErr) {
				_ = WriteJSON(w, apiErr.StatusCode, apiErr)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(err.Error()))
			}
		}
	}
}
