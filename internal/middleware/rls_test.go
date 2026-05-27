package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestApplyRLSContext(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	identityID := uuid.New()
	organization := db.Organization{ID: orgID}
	identity := IdentityContext{
		IdentityID:     identityID,
		Type:           string(util.IdentityUser),
		ActorID:        uuid.New(),
		OrganizationID: orgID,
		Name:           "Merchant User",
		Email:          "merchant@example.com",
		Service:        util.SessionServiceMerchantPanel,
		SessionID:      uuid.New(),
	}

	t.Run("adds RLS context", func(t *testing.T) {
		t.Parallel()

		m := New(&mockStore{}, nil)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		ctx := context.WithValue(req.Context(), IdentityContextKey{}, identity)
		ctx = context.WithValue(ctx, OrganizationContextKey{}, organization)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler := m.ApplyRLSContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rls, ok := db.RLSContextFromContext(r.Context())
			require.True(t, ok)
			require.Equal(t, identityID, rls.IdentityID)
			require.Equal(t, orgID, rls.OrganizationID)
			require.False(t, rls.IsPlatformUser)
			require.False(t, rls.IsPlatformAdmin)
			w.WriteHeader(http.StatusNoContent)
		}))

		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("marks admin panel identity as platform", func(t *testing.T) {
		t.Parallel()

		adminIdentity := identity
		adminIdentity.Service = util.SessionServiceAdminPanel
		m := New(&mockStore{}, nil)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		ctx := context.WithValue(req.Context(), IdentityContextKey{}, adminIdentity)
		ctx = context.WithValue(ctx, OrganizationContextKey{}, organization)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler := m.ApplyRLSContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rls, ok := db.RLSContextFromContext(r.Context())
			require.True(t, ok)
			require.True(t, rls.IsPlatformUser)
			require.True(t, rls.IsPlatformAdmin)
			w.WriteHeader(http.StatusNoContent)
		}))

		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("requires identity context", func(t *testing.T) {
		t.Parallel()

		m := New(&mockStore{}, nil)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req = req.WithContext(context.WithValue(req.Context(), OrganizationContextKey{}, organization))
		rec := httptest.NewRecorder()

		handler := m.ApplyRLSContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("should not be called")
		}))

		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("requires organization context", func(t *testing.T) {
		t.Parallel()

		m := New(&mockStore{}, nil)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req = req.WithContext(context.WithValue(req.Context(), IdentityContextKey{}, identity))
		rec := httptest.NewRecorder()

		handler := m.ApplyRLSContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("should not be called")
		}))

		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("rejects organization mismatch", func(t *testing.T) {
		t.Parallel()

		mismatchedOrg := db.Organization{ID: uuid.New()}
		m := New(&mockStore{}, nil)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		ctx := context.WithValue(req.Context(), IdentityContextKey{}, identity)
		ctx = context.WithValue(ctx, OrganizationContextKey{}, mismatchedOrg)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler := m.ApplyRLSContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("should not be called")
		}))

		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code)
	})
}
