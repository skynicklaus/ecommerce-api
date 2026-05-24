package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/internal/middleware"
	"github.com/skynicklaus/ecommerce-api/internal/password"
	"github.com/skynicklaus/ecommerce-api/util"
)

// loginTimingSentinel is a pre-computed argon2id hash used to normalize response
// time when a login email is not found, preventing timing-based account enumeration.
//
//nolint:gochecknoglobals // timing sentinel: pre-computed once at startup, never changes
var loginTimingSentinel = mustComputeTimingSentinel()

func mustComputeTimingSentinel() string {
	h, err := password.HashPassword("sentinel-value-never-matches-a-real-credential")
	if err != nil {
		panic("failed to initialize login timing sentinel: " + err.Error())
	}
	return h
}

type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

type LoginResponse struct {
	Token     string                     `json:"token"`
	ExpiresAt time.Time                  `json:"expiresAt"`
	Identity  middleware.IdentityContext `json:"identity"`
}

// LoginCustomer handles buyer logins for service = 'buyer_platform'.
func (h *V1Handler) LoginCustomer(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var req LoginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	res, err := h.store.GetCustomerWithCredential(ctx, req.Email)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = password.CheckPassword(req.Password, loginTimingSentinel)
			return apierror.NewAPIError(http.StatusUnauthorized, errors.New("invalid credentials"))
		}
		return err
	}

	if res.HashedPassword == nil {
		_ = password.CheckPassword(req.Password, loginTimingSentinel)
		return apierror.NewAPIError(http.StatusUnauthorized, errors.New("invalid credentials"))
	}

	if checkErr := password.CheckPassword(req.Password, *res.HashedPassword); checkErr != nil {
		if !errors.Is(checkErr, password.ErrMismatchedHashAndPassword) {
			return fmt.Errorf("password check failed: %w", checkErr)
		}
		return apierror.NewAPIError(http.StatusUnauthorized, errors.New("invalid credentials"))
	}

	var orgID *uuid.UUID
	member, memberErr := h.store.GetMemberByIdentityID(ctx, res.IdentityID)
	switch {
	case memberErr == nil:
		orgID = &member.OrganizationID
	case errors.Is(memberErr, db.ErrNotFound):
		// Individual org not yet created — handle gracefully.
	default:
		return fmt.Errorf("failed to resolve membership: %w", memberErr)
	}

	token, expires, sessionID, dbErr := h.createStatefulSession(
		ctx,
		res.IdentityID,
		orgID,
		util.SessionServiceBuyerPlatform,
		r,
	)
	if dbErr != nil {
		return dbErr
	}

	resolvedOrgID := uuid.Nil
	if orgID != nil {
		resolvedOrgID = *orgID
	}

	return WriteJSON(w, http.StatusOK, LoginResponse{
		Token:     token,
		ExpiresAt: expires,
		Identity: middleware.IdentityContext{
			IdentityID:     res.IdentityID,
			Type:           string(util.IdentityCustomer),
			ActorID:        res.ID,
			OrganizationID: resolvedOrgID,
			Name:           res.Name,
			Email:          res.Email,
			Service:        util.SessionServiceBuyerPlatform,
			SessionID:      sessionID,
		},
	})
}

// LoginMerchant handles merchant operator logins for service = 'merchant_panel'.
func (h *V1Handler) LoginMerchant(w http.ResponseWriter, r *http.Request) error {
	return h.handleUserLogin(w, r, util.SessionServiceMerchantPanel)
}

// LoginAdmin handles system administrator logins for service = 'admin_panel'.
func (h *V1Handler) LoginAdmin(w http.ResponseWriter, r *http.Request) error {
	return h.handleUserLogin(w, r, util.SessionServiceAdminPanel)
}

func (h *V1Handler) handleUserLogin(
	w http.ResponseWriter,
	r *http.Request,
	service util.SessionService,
) error {
	ctx := r.Context()
	var req LoginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	res, err := h.store.GetUserWithCredential(ctx, req.Email)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			_ = password.CheckPassword(req.Password, loginTimingSentinel)
			return apierror.NewAPIError(http.StatusUnauthorized, errors.New("invalid credentials"))
		}
		return err
	}

	if res.HashedPassword == nil {
		_ = password.CheckPassword(req.Password, loginTimingSentinel)
		return apierror.NewAPIError(http.StatusUnauthorized, errors.New("invalid credentials"))
	}

	if checkErr := password.CheckPassword(req.Password, *res.HashedPassword); checkErr != nil {
		if !errors.Is(checkErr, password.ErrMismatchedHashAndPassword) {
			return fmt.Errorf("password check failed: %w", checkErr)
		}
		return apierror.NewAPIError(http.StatusUnauthorized, errors.New("invalid credentials"))
	}

	var orgID *uuid.UUID
	member, memberErr := h.store.GetMemberByIdentityID(ctx, res.IdentityID)
	switch {
	case memberErr == nil:
		orgID = &member.OrganizationID
	case errors.Is(memberErr, db.ErrNotFound):
		// No membership yet — valid for platform admins or users pending org assignment.
	default:
		return fmt.Errorf("failed to resolve membership: %w", memberErr)
	}

	token, expires, sessionID, dbErr := h.createStatefulSession(
		ctx,
		res.IdentityID,
		orgID,
		service,
		r,
	)
	if dbErr != nil {
		return dbErr
	}

	resolvedOrgID := uuid.Nil
	if orgID != nil {
		resolvedOrgID = *orgID
	}

	return WriteJSON(w, http.StatusOK, LoginResponse{
		Token:     token,
		ExpiresAt: expires,
		Identity: middleware.IdentityContext{
			IdentityID:     res.IdentityID,
			Type:           string(util.IdentityUser),
			ActorID:        res.ID,
			OrganizationID: resolvedOrgID,
			Name:           res.Name,
			Email:          res.Email,
			Service:        service,
			SessionID:      sessionID,
		},
	})
}

const (
	sessionTokenBytes = 32
	sessionMessageKey = "message"
)

func (h *V1Handler) createStatefulSession(
	ctx context.Context,
	identityID uuid.UUID,
	orgID *uuid.UUID,
	service util.SessionService,
	r *http.Request,
) (string, time.Time, uuid.UUID, error) {
	raw := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, uuid.Nil, fmt.Errorf("failed to generate session: %w", err)
	}
	tokenStr := hex.EncodeToString(raw)

	expires := time.Now().Add(util.SessionTTL)
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr // fallback if RemoteAddr has no port (e.g. unix socket)
	}
	ua := r.UserAgent()

	session, dbErr := h.store.CreateSession(ctx, db.CreateSessionParams{
		IdentityID: identityID,
		Token: util.HashSessionToken(
			tokenStr,
		), // store hash; raw token returned to client only
		Service:              string(service),
		ExpiresAt:            expires,
		IpAddress:            &ip,
		UserAgent:            &ua,
		ActiveOrganizationID: orgID,
	})
	if dbErr != nil {
		return "", time.Time{}, uuid.Nil, fmt.Errorf("failed to save session: %w", dbErr)
	}

	return tokenStr, expires, session.ID, nil
}

// Logout evicts the token immediately from the PostgreSQL database using context lookups.
func (h *V1Handler) Logout(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	// Read token directly from context (populated during RequireService middleware run)
	token, err := middleware.GetTokenFromContext(ctx)
	if err != nil {
		return apierror.NewAPIError(http.StatusUnauthorized, err)
	}

	if err = h.store.DeleteSessionByToken(ctx, token); err != nil {
		return fmt.Errorf("failed to delete session on logout: %w", err)
	}

	return WriteJSON(
		w,
		http.StatusOK,
		map[string]string{sessionMessageKey: "logged out successfully"},
	)
}

// GetMe prints the active identity context.
func (h *V1Handler) GetMe(w http.ResponseWriter, r *http.Request) error {
	identity, err := middleware.GetIdentityFromContext(r.Context())
	if err != nil {
		return apierror.NewAPIError(http.StatusUnauthorized, err)
	}
	return WriteJSON(w, http.StatusOK, identity)
}

type SessionMetadataResponse struct {
	ID        uuid.UUID `json:"id"`
	Service   string    `json:"service"`
	IPAddress *string   `json:"ipAddress"`
	UserAgent *string   `json:"userAgent"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	IsCurrent bool      `json:"isCurrent"`
}

// ListActiveSessions returns all valid sessions for the active identity.
func (h *V1Handler) ListActiveSessions(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	identity, err := middleware.GetIdentityFromContext(ctx)
	if err != nil {
		return apierror.NewAPIError(http.StatusUnauthorized, err)
	}

	sessions, dbErr := h.store.ListSessionsByIdentity(ctx, db.ListSessionsByIdentityParams{
		IdentityID: identity.IdentityID,
		Service:    string(identity.Service),
	})
	if dbErr != nil {
		return fmt.Errorf("failed to fetch active sessions: %w", dbErr)
	}

	resp := make([]SessionMetadataResponse, len(sessions))
	for i, s := range sessions {
		resp[i] = SessionMetadataResponse{
			ID:        s.ID,
			Service:   s.Service,
			IPAddress: s.IpAddress,
			UserAgent: s.UserAgent,
			CreatedAt: s.CreatedAt,
			UpdatedAt: s.UpdatedAt,
			ExpiresAt: s.ExpiresAt,
			IsCurrent: s.ID == identity.SessionID,
		}
	}

	return WriteJSON(w, http.StatusOK, resp)
}

// RevokeOtherSessions signs out of every other device (keeps the active one).
func (h *V1Handler) RevokeOtherSessions(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	identity, err := middleware.GetIdentityFromContext(ctx)
	if err != nil {
		return apierror.NewAPIError(http.StatusUnauthorized, err)
	}

	token, tokenErr := middleware.GetTokenFromContext(ctx)
	if tokenErr != nil {
		return apierror.NewAPIError(http.StatusUnauthorized, tokenErr)
	}

	dbErr := h.store.DeleteAllOtherSessionsByIdentity(
		ctx,
		db.DeleteAllOtherSessionsByIdentityParams{
			IdentityID: identity.IdentityID,
			Service:    string(identity.Service),
			Token:      token,
		},
	)
	if dbErr != nil {
		return fmt.Errorf("failed to revoke other sessions: %w", dbErr)
	}

	return WriteJSON(
		w,
		http.StatusOK,
		map[string]string{sessionMessageKey: "successfully signed out of all other sessions"},
	)
}

// RevokeSessionByID logs out a specific session ID belonging to the user.
func (h *V1Handler) RevokeSessionByID(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	identity, err := middleware.GetIdentityFromContext(ctx)
	if err != nil {
		return apierror.NewAPIError(http.StatusUnauthorized, err)
	}

	sessionIDStr := chi.URLParam(r, "id")
	sessionID, parseErr := uuid.Parse(sessionIDStr)
	if parseErr != nil {
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("invalid session ID format"))
	}

	_, dbErr := h.store.DeleteSessionByIDAndIdentity(ctx, db.DeleteSessionByIDAndIdentityParams{
		ID:         sessionID,
		IdentityID: identity.IdentityID,
	})
	if dbErr != nil {
		if errors.Is(dbErr, db.ErrNotFound) {
			return apierror.NewAPIError(http.StatusNotFound, errors.New("session not found"))
		}
		return fmt.Errorf("failed to revoke session: %w", dbErr)
	}

	return WriteJSON(
		w,
		http.StatusOK,
		map[string]string{sessionMessageKey: "session revoked successfully"},
	)
}
