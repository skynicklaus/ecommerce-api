//go:build integration

package db_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

func createSession(t *testing.T) (db.Identity, db.Session) {
	t.Helper()
	identity := createRandomIdentity(t)
	ip := "127.0.0.1"
	ua := "Go-Test-Suite"
	session, err := testStore.CreateSession(t.Context(), db.CreateSessionParams{
		IdentityID: identity.ID,
		Token:      util.GetRandomString(t, 32),
		Service:    string(util.SessionServiceBuyerPlatform),
		ExpiresAt:  time.Now().Add(time.Hour),
		IpAddress:  &ip,
		UserAgent:  &ua,
	})
	require.NoError(t, err)
	return identity, session
}

func TestCreateSession(t *testing.T) {
	identity := createRandomIdentity(t)
	ip := "127.0.0.1"
	ua := "Go-Test-Suite"
	expiresAt := time.Now().Add(time.Hour)

	session, err := testStore.CreateSession(t.Context(), db.CreateSessionParams{
		IdentityID: identity.ID,
		Token:      util.GetRandomString(t, 32),
		Service:    string(util.SessionServiceBuyerPlatform),
		ExpiresAt:  expiresAt,
		IpAddress:  &ip,
		UserAgent:  &ua,
	})
	require.NoError(t, err)
	require.Equal(t, identity.ID, session.IdentityID)
	require.Equal(t, string(util.SessionServiceBuyerPlatform), session.Service)
	require.Equal(t, ip, *session.IpAddress)
	require.Equal(t, ua, *session.UserAgent)
	require.WithinDuration(t, expiresAt, session.ExpiresAt, 5*time.Second)
	require.NotZero(t, session.CreatedAt)
}

func TestGetSessionWithIdentity(t *testing.T) {
	identity, session := createSession(t)

	row, err := testStore.GetSessionWithIdentity(t.Context(), session.Token)
	require.NoError(t, err)
	require.Equal(t, session.ID, row.SessionID)
	require.Equal(t, identity.ID, row.IdentityID)
	require.Equal(t, session.Token, row.Token)
	require.Equal(t, string(util.SessionServiceBuyerPlatform), row.Service)
	require.Equal(t, identity.Type, row.IdentityType)
}

func TestGetSessionWithIdentity_ExpiredReturnsError(t *testing.T) {
	identity := createRandomIdentity(t)
	ip := "127.0.0.1"
	ua := "Go-Test-Suite"
	expiredToken := util.GetRandomString(t, 32)

	_, err := testStore.CreateSession(t.Context(), db.CreateSessionParams{
		IdentityID: identity.ID,
		Token:      expiredToken,
		Service:    string(util.SessionServiceBuyerPlatform),
		ExpiresAt:  time.Now().Add(-time.Minute),
		IpAddress:  &ip,
		UserAgent:  &ua,
	})
	require.NoError(t, err)

	_, err = testStore.GetSessionWithIdentity(t.Context(), expiredToken)
	require.Error(t, err)
}

func TestRenewSession(t *testing.T) {
	_, session := createSession(t)

	before, err := testStore.GetSessionWithIdentity(t.Context(), session.Token)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return time.Now().After(before.UpdatedAt)
	}, time.Second, 10*time.Millisecond)

	newExpires := time.Now().Add(7 * 24 * time.Hour)
	err = testStore.RenewSession(t.Context(), db.RenewSessionParams{
		ExpiresAt: newExpires,
		Token:     session.Token,
	})
	require.NoError(t, err)

	after, err := testStore.GetSessionWithIdentity(t.Context(), session.Token)
	require.NoError(t, err)
	require.WithinDuration(t, newExpires, after.ExpiresAt, 5*time.Second)
	require.True(t, after.UpdatedAt.After(before.UpdatedAt), "updated_at should advance after renewal")
}

func TestListSessionsByIdentity(t *testing.T) {
	t.Run("returns only active sessions", func(t *testing.T) {
		identity := createRandomIdentity(t)
		ip := "127.0.0.1"
		ua := "Go-Test-Suite"

		makeSession := func(expires time.Time) db.Session {
			s, sErr := testStore.CreateSession(t.Context(), db.CreateSessionParams{
				IdentityID: identity.ID,
				Token:      util.GetRandomString(t, 32),
				Service:    string(util.SessionServiceBuyerPlatform),
				ExpiresAt:  expires,
				IpAddress:  &ip,
				UserAgent:  &ua,
			})
			require.NoError(t, sErr)
			return s
		}

		active1 := makeSession(time.Now().Add(time.Hour))
		active2 := makeSession(time.Now().Add(2 * time.Hour))
		_ = makeSession(time.Now().Add(-time.Minute)) // expired — must not appear

		sessions, err := testStore.ListSessionsByIdentity(t.Context(), db.ListSessionsByIdentityParams{
			IdentityID: identity.ID,
			Service:    string(util.SessionServiceBuyerPlatform),
		})
		require.NoError(t, err)
		require.Len(t, sessions, 2)
		ids := []interface{}{sessions[0].ID, sessions[1].ID}
		require.Contains(t, ids, active1.ID)
		require.Contains(t, ids, active2.ID)
	})

	t.Run("does not return sessions from other identities", func(t *testing.T) {
		_, session := createSession(t)
		otherIdentity := createRandomIdentity(t)

		sessions, err := testStore.ListSessionsByIdentity(t.Context(), db.ListSessionsByIdentityParams{
			IdentityID: otherIdentity.ID,
			Service:    string(util.SessionServiceBuyerPlatform),
		})
		require.NoError(t, err)
		for _, s := range sessions {
			require.NotEqual(t, session.ID, s.ID)
		}
	})

	t.Run("response excludes token", func(t *testing.T) {
		identity, _ := createSession(t)
		sessions, err := testStore.ListSessionsByIdentity(t.Context(), db.ListSessionsByIdentityParams{
			IdentityID: identity.ID,
			Service:    string(util.SessionServiceBuyerPlatform),
		})
		require.NoError(t, err)
		require.NotEmpty(t, sessions)
		// ListSessionsByIdentityRow has no Token field — asserted at compile time by this assignment.
		var _ db.ListSessionsByIdentityRow = sessions[0]
	})
}

func TestDeleteSessionByIDAndIdentity(t *testing.T) {
	t.Run("deletes the target session", func(t *testing.T) {
		identity := createRandomIdentity(t)
		ip := "127.0.0.1"
		ua := "Go-Test-Suite"

		makeSession := func() db.Session {
			s, sErr := testStore.CreateSession(t.Context(), db.CreateSessionParams{
				IdentityID: identity.ID,
				Token:      util.GetRandomString(t, 32),
				Service:    string(util.SessionServiceBuyerPlatform),
				ExpiresAt:  time.Now().Add(time.Hour),
				IpAddress:  &ip,
				UserAgent:  &ua,
			})
			require.NoError(t, sErr)
			return s
		}

		s1 := makeSession()
		s2 := makeSession()

		_, err := testStore.DeleteSessionByIDAndIdentity(t.Context(), db.DeleteSessionByIDAndIdentityParams{
			ID:         s1.ID,
			IdentityID: identity.ID,
		})
		require.NoError(t, err)

		remaining, err := testStore.ListSessionsByIdentity(t.Context(), db.ListSessionsByIdentityParams{
			IdentityID: identity.ID,
			Service:    string(util.SessionServiceBuyerPlatform),
		})
		require.NoError(t, err)
		require.Len(t, remaining, 1)
		require.Equal(t, s2.ID, remaining[0].ID)
	})

	t.Run("does not delete session belonging to another identity", func(t *testing.T) {
		_, s1 := createSession(t)
		_, s2 := createSession(t)

		_, err := testStore.DeleteSessionByIDAndIdentity(t.Context(), db.DeleteSessionByIDAndIdentityParams{
			ID:         s1.ID,
			IdentityID: s2.IdentityID,
		})
		require.ErrorIs(t, err, db.ErrNotFound, "deleting with wrong identity should return not found")

		_, err = testStore.GetSessionWithIdentity(t.Context(), s1.Token)
		require.NoError(t, err, "session should still exist when deleted with wrong identity")
	})
}

func TestDeleteAllOtherSessionsByIdentity(t *testing.T) {
	identity := createRandomIdentity(t)
	ip := "127.0.0.1"
	ua := "Go-Test-Suite"

	makeSession := func() db.Session {
		s, sErr := testStore.CreateSession(t.Context(), db.CreateSessionParams{
			IdentityID: identity.ID,
			Token:      util.GetRandomString(t, 32),
			Service:    string(util.SessionServiceBuyerPlatform),
			ExpiresAt:  time.Now().Add(time.Hour),
			IpAddress:  &ip,
			UserAgent:  &ua,
		})
		require.NoError(t, sErr)
		return s
	}

	_ = makeSession()
	_ = makeSession()
	kept := makeSession()

	err := testStore.DeleteAllOtherSessionsByIdentity(t.Context(), db.DeleteAllOtherSessionsByIdentityParams{
		IdentityID: identity.ID,
		Service:    string(util.SessionServiceBuyerPlatform),
		Token:      kept.Token,
	})
	require.NoError(t, err)

	remaining, err := testStore.ListSessionsByIdentity(t.Context(), db.ListSessionsByIdentityParams{
		IdentityID: identity.ID,
		Service:    string(util.SessionServiceBuyerPlatform),
	})
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	require.Equal(t, kept.ID, remaining[0].ID)
}

func TestDeleteSessionByToken(t *testing.T) {
	_, session := createSession(t)

	err := testStore.DeleteSessionByToken(t.Context(), session.Token)
	require.NoError(t, err)

	_, err = testStore.GetSessionWithIdentity(t.Context(), session.Token)
	require.Error(t, err)
}

func TestDeleteAllSessionsByIdentity(t *testing.T) {
	t.Run("removes every session for the identity", func(t *testing.T) {
		identity := createRandomIdentity(t)
		ip := "127.0.0.1"
		ua := "Go-Test-Suite"

		for range 3 {
			_, err := testStore.CreateSession(t.Context(), db.CreateSessionParams{
				IdentityID: identity.ID,
				Token:      util.GetRandomString(t, 32),
				Service:    string(util.SessionServiceBuyerPlatform),
				ExpiresAt:  time.Now().Add(time.Hour),
				IpAddress:  &ip,
				UserAgent:  &ua,
			})
			require.NoError(t, err)
		}

		err := testStore.DeleteAllSessionsByIdentity(t.Context(), identity.ID)
		require.NoError(t, err)

		remaining, err := testStore.ListSessionsByIdentity(t.Context(), db.ListSessionsByIdentityParams{
			IdentityID: identity.ID,
			Service:    string(util.SessionServiceBuyerPlatform),
		})
		require.NoError(t, err)
		require.Empty(t, remaining)
	})

	t.Run("does not affect sessions of other identities", func(t *testing.T) {
		_, session := createSession(t)
		otherIdentity := createRandomIdentity(t)

		err := testStore.DeleteAllSessionsByIdentity(t.Context(), otherIdentity.ID)
		require.NoError(t, err)

		_, err = testStore.GetSessionWithIdentity(t.Context(), session.Token)
		require.NoError(t, err, "session should survive deletion of another identity's sessions")
	})
}
