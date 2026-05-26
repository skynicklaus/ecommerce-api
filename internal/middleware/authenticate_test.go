package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

type mockStore struct {
	db.Store

	getSessionWithIdentityFunc  func(ctx context.Context, token string) (db.GetSessionWithIdentityRow, error)
	renewSessionFunc            func(ctx context.Context, arg db.RenewSessionParams) error
	getCustomerByIdentityIDFunc func(ctx context.Context, identityID uuid.UUID) (db.Customer, error)
	getUserByIdentityIDFunc     func(ctx context.Context, identityID uuid.UUID) (db.User, error)
}

func (m *mockStore) GetSessionWithIdentity(ctx context.Context, token string) (db.GetSessionWithIdentityRow, error) {
	if m.getSessionWithIdentityFunc != nil {
		return m.getSessionWithIdentityFunc(ctx, token)
	}
	return db.GetSessionWithIdentityRow{}, errors.New("unimplemented")
}

func (m *mockStore) RenewSession(ctx context.Context, arg db.RenewSessionParams) error {
	if m.renewSessionFunc != nil {
		return m.renewSessionFunc(ctx, arg)
	}
	return errors.New("unimplemented")
}

func (m *mockStore) GetCustomerByIdentityID(ctx context.Context, identityID uuid.UUID) (db.Customer, error) {
	if m.getCustomerByIdentityIDFunc != nil {
		return m.getCustomerByIdentityIDFunc(ctx, identityID)
	}
	return db.Customer{}, errors.New("unimplemented")
}

func (m *mockStore) GetUserByIdentityID(ctx context.Context, identityID uuid.UUID) (db.User, error) {
	if m.getUserByIdentityIDFunc != nil {
		return m.getUserByIdentityIDFunc(ctx, identityID)
	}
	return db.User{}, errors.New("unimplemented")
}

func generateRandomHex(t *testing.T, size int) string {
	t.Helper()
	b := make([]byte, size)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return hex.EncodeToString(b)
}

func TestRequireService(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	sessionID := uuid.New()
	identityID := uuid.New()
	customerID := uuid.New()

	baseSession := db.GetSessionWithIdentityRow{
		SessionID:            sessionID,
		IdentityID:           identityID,
		IdentityType:         string(util.IdentityCustomer),
		Service:              string(util.SessionServiceBuyerPlatform),
		ActiveOrganizationID: &orgID,
		CreatedAt:            time.Now(),
		ExpiresAt:            time.Now().Add(5 * 24 * time.Hour), // Plenty of TTL
		UpdatedAt:            time.Now(),
	}

	mockCust := db.Customer{
		ID:         customerID,
		IdentityID: identityID,
		Name:       "John Doe",
		Email:      "john@example.com",
	}

	t.Run("missing authorization header", func(t *testing.T) {
		t.Parallel()
		store := &mockStore{}
		m := New(store, nil)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler := m.RequireService(util.SessionServiceBuyerPlatform)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("should not be called")
			}),
		)

		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid header format", func(t *testing.T) {
		t.Parallel()
		store := &mockStore{}
		m := New(store, nil)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "InvalidFormat token")
		rec := httptest.NewRecorder()

		handler := m.RequireService(util.SessionServiceBuyerPlatform)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("should not be called")
			}),
		)

		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid token format length", func(t *testing.T) {
		t.Parallel()
		store := &mockStore{}
		m := New(store, nil)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer short")
		rec := httptest.NewRecorder()

		handler := m.RequireService(util.SessionServiceBuyerPlatform)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("should not be called")
			}),
		)

		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("session not found", func(t *testing.T) {
		t.Parallel()
		store := &mockStore{
			getSessionWithIdentityFunc: func(ctx context.Context, token string) (db.GetSessionWithIdentityRow, error) {
				return db.GetSessionWithIdentityRow{}, db.ErrNotFound
			},
		}
		m := New(store, nil)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		token := generateRandomHex(t, 32)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		handler := m.RequireService(util.SessionServiceBuyerPlatform)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("should not be called")
			}),
		)

		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("unauthorized service access", func(t *testing.T) {
		t.Parallel()
		store := &mockStore{
			getSessionWithIdentityFunc: func(ctx context.Context, token string) (db.GetSessionWithIdentityRow, error) {
				sess := baseSession
				sess.Service = string(util.SessionServiceMerchantPanel) // Request buyer, get merchant
				return sess, nil
			},
		}
		m := New(store, nil)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		token := generateRandomHex(t, 32)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		handler := m.RequireService(util.SessionServiceBuyerPlatform)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("should not be called")
			}),
		)

		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("successful authentication without sliding renewal", func(t *testing.T) {
		t.Parallel()
		store := &mockStore{
			getSessionWithIdentityFunc: func(ctx context.Context, token string) (db.GetSessionWithIdentityRow, error) {
				return baseSession, nil
			},
			getCustomerByIdentityIDFunc: func(ctx context.Context, id uuid.UUID) (db.Customer, error) {
				return mockCust, nil
			},
		}
		m := New(store, nil)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		token := generateRandomHex(t, 32)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		var called bool
		handler := m.RequireService(util.SessionServiceBuyerPlatform)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				identity, err := GetIdentityFromContext(r.Context())
				require.NoError(t, err)
				require.Equal(t, customerID, identity.ActorID)
				require.Equal(t, "John Doe", identity.Name)

				tok, err := GetTokenFromContext(r.Context())
				require.NoError(t, err)
				require.Equal(t, util.HashSessionToken(token), tok)
			}),
		)

		handler.ServeHTTP(rec, req)
		require.True(t, called)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, baseSession.ExpiresAt.UTC().Format(time.RFC3339), rec.Header().Get("X-Session-Expires-At"))
	})

	t.Run("sliding renewal trigger", func(t *testing.T) {
		t.Parallel()

		renewCalled := make(chan struct{}, 1)
		store := &mockStore{
			getSessionWithIdentityFunc: func(ctx context.Context, token string) (db.GetSessionWithIdentityRow, error) {
				sess := baseSession
				// Remaining TTL is 1 day (which is < SessionTTL / 2 i.e. 3.5 days)
				sess.ExpiresAt = time.Now().Add(24 * time.Hour)
				return sess, nil
			},
			getCustomerByIdentityIDFunc: func(ctx context.Context, id uuid.UUID) (db.Customer, error) {
				return mockCust, nil
			},
			renewSessionFunc: func(ctx context.Context, arg db.RenewSessionParams) error {
				close(renewCalled)
				return nil
			},
		}
		m := New(store, nil)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		token := generateRandomHex(t, 32)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		handler := m.RequireService(util.SessionServiceBuyerPlatform)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		)

		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		// Wait for background renewal goroutine to complete
		select {
		case <-renewCalled:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("background renewal was not triggered")
		}

		// Response header should match the new sliding expires time
		headerExpires := rec.Header().Get("X-Session-Expires-At")
		require.NotEmpty(t, headerExpires)
		parsed, err := time.Parse(time.RFC3339, headerExpires)
		require.NoError(t, err)
		require.WithinDuration(t, time.Now().Add(util.SessionTTL), parsed, 5*time.Second)
	})

	t.Run("concurrent renewal requests are debounced", func(t *testing.T) {
		t.Parallel()

		var renewalCount int32
		var wg sync.WaitGroup
		renewStarted := make(chan struct{})
		releaseRenew := make(chan struct{})
		var closeRenewStarted sync.Once

		store := &mockStore{
			getSessionWithIdentityFunc: func(ctx context.Context, token string) (db.GetSessionWithIdentityRow, error) {
				sess := baseSession
				sess.ExpiresAt = time.Now().Add(1 * time.Hour) // meets sliding renewal threshold
				return sess, nil
			},
			getCustomerByIdentityIDFunc: func(ctx context.Context, id uuid.UUID) (db.Customer, error) {
				return mockCust, nil
			},
			renewSessionFunc: func(ctx context.Context, arg db.RenewSessionParams) error {
				closeRenewStarted.Do(func() { close(renewStarted) })
				<-releaseRenew
				atomic.AddInt32(&renewalCount, 1)
				return nil
			},
		}
		m := New(store, nil)
		token := generateRandomHex(t, 32)

		numConcurrent := 5
		wg.Add(numConcurrent)

		for i := 0; i < numConcurrent; i++ {
			go func() {
				defer wg.Done()
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()

				handler := m.RequireService(util.SessionServiceBuyerPlatform)(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
				)
				handler.ServeHTTP(rec, req)
				require.Equal(t, http.StatusOK, rec.Code)
			}()
		}

		wg.Wait()

		select {
		case <-renewStarted:
		case <-time.After(time.Second):
			t.Fatal("background renewal was not triggered")
		}
		close(releaseRenew)

		require.Eventually(t, func() bool {
			return atomic.LoadInt32(&renewalCount) == 1
		}, time.Second, 10*time.Millisecond)
		require.Equal(t, int32(1), atomic.LoadInt32(&renewalCount), "concurrent renewal calls should be debounced to 1 call")
	})

	t.Run("absolute session ceiling enforced", func(t *testing.T) {
		t.Parallel()

		renewCalled := make(chan struct{}, 1)
		store := &mockStore{
			getSessionWithIdentityFunc: func(ctx context.Context, token string) (db.GetSessionWithIdentityRow, error) {
				sess := baseSession
				// Created 89 days ago (buyer max is 90 days)
				sess.CreatedAt = time.Now().Add(-89 * 24 * time.Hour)
				// Remaining TTL is 1 hour
				sess.ExpiresAt = time.Now().Add(1 * time.Hour)
				return sess, nil
			},
			getCustomerByIdentityIDFunc: func(ctx context.Context, id uuid.UUID) (db.Customer, error) {
				return mockCust, nil
			},
			renewSessionFunc: func(ctx context.Context, arg db.RenewSessionParams) error {
				close(renewCalled)
				return nil
			},
		}
		m := New(store, nil)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		token := generateRandomHex(t, 32)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		handler := m.RequireService(util.SessionServiceBuyerPlatform)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		)

		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		// Wait for background renewal
		select {
		case <-renewCalled:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("background renewal was not triggered")
		}

		// The new sliding expires time MUST NOT exceed the absolute ceiling
		// CreatedAt + 90 days. Since CreatedAt was 89 days ago, it can slide at most 1 day!
		headerExpires := rec.Header().Get("X-Session-Expires-At")
		require.NotEmpty(t, headerExpires)
		parsed, err := time.Parse(time.RFC3339, headerExpires)
		require.NoError(t, err)

		expectedCeiling := baseSession.CreatedAt.Add(-89 * 24 * time.Hour).Add(util.AbsoluteSessionMax(util.SessionServiceBuyerPlatform))
		require.WithinDuration(t, expectedCeiling, parsed, 5*time.Second)
	})
}
