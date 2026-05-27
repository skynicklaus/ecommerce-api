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
	Email    string `json:"email"    validate:"required,email,max=254" example:"merchant@example.com"`
	Password string `json:"password" validate:"required,min=8,max=72" example:"correct-horse-battery-staple"`
}

type LoginResponse struct {
	Token     string                     `json:"token"`
	ExpiresAt time.Time                  `json:"expiresAt"`
	Identity  middleware.IdentityContext `json:"identity"`
}

// LoginCustomer godoc
//
//	@Summary		Customer login
//	@Description	Authenticates a customer and creates a buyer-platform session.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		LoginRequest	true	"Login credentials"
//	@Success		200		{object}	LoginResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Router			/auth/customer/login [post]
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

// LoginMerchant godoc
//
//	@Summary		Merchant login
//	@Description	Authenticates a merchant user and creates a merchant-panel session.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		LoginRequest	true	"Login credentials"
//	@Success		200		{object}	LoginResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Router			/auth/merchant/login [post]
func (h *V1Handler) LoginMerchant(w http.ResponseWriter, r *http.Request) error {
	return h.handleUserLogin(w, r, util.SessionServiceMerchantPanel)
}

// LoginAdmin godoc
//
//	@Summary		Admin login
//	@Description	Authenticates a platform administrator and creates an admin-panel session.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		LoginRequest	true	"Login credentials"
//	@Success		200		{object}	LoginResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Router			/auth/admin/login [post]
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

	orgID, orgErr := h.authorizedOrganizationForUserLogin(ctx, res.IdentityID, service)
	if orgErr != nil {
		return orgErr
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

	return WriteJSON(w, http.StatusOK, LoginResponse{
		Token:     token,
		ExpiresAt: expires,
		Identity: middleware.IdentityContext{
			IdentityID:     res.IdentityID,
			Type:           string(util.IdentityUser),
			ActorID:        res.ID,
			OrganizationID: *orgID,
			Name:           res.Name,
			Email:          res.Email,
			Service:        service,
			SessionID:      sessionID,
		},
	})
}

func (h *V1Handler) authorizedOrganizationForUserLogin(
	ctx context.Context,
	identityID uuid.UUID,
	service util.SessionService,
) (*uuid.UUID, error) {
	member, memberErr := h.store.GetMemberByIdentityID(ctx, identityID)
	if memberErr != nil {
		if errors.Is(memberErr, db.ErrNotFound) {
			return nil, apierror.NewAPIError(http.StatusUnauthorized, errors.New("invalid credentials"))
		}
		return nil, fmt.Errorf("failed to resolve membership: %w", memberErr)
	}

	org, orgErr := h.store.GetOrganizationByID(ctx, member.OrganizationID)
	if orgErr != nil {
		if errors.Is(orgErr, db.ErrNotFound) {
			return nil, apierror.NewAPIError(http.StatusUnauthorized, errors.New("invalid credentials"))
		}
		return nil, fmt.Errorf("failed to resolve organization: %w", orgErr)
	}

	if !organizationAllowedForService(org.Type, service) {
		return nil, apierror.NewAPIError(http.StatusUnauthorized, errors.New("invalid credentials"))
	}

	return &member.OrganizationID, nil
}

func organizationAllowedForService(orgType string, service util.SessionService) bool {
	switch service {
	case util.SessionServiceAdminPanel:
		return orgType == string(util.OrganizationTypePlatform)
	case util.SessionServiceMerchantPanel:
		return orgType == string(util.OrganizationTypeMerchant) ||
			orgType == string(util.OrganizationTypeCompany)
	default:
		return false
	}
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

// Logout godoc
//
//	@Summary		Logout current session
//	@Description	Revokes the active session token for the current service surface.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	MessageResponse
//	@Failure		401	{object}	apierror.APIError
//	@Failure		500	{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/auth/customer/logout [post]
//	@Router			/auth/merchant/logout [post]
//	@Router			/auth/admin/logout [post]
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

// GetMe godoc
//
//	@Summary		Get current identity
//	@Description	Returns the authenticated identity resolved from the active session.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	middleware.IdentityContext
//	@Failure		401	{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/auth/customer/me [get]
//	@Router			/auth/merchant/me [get]
//	@Router			/auth/admin/me [get]
func (h *V1Handler) GetMe(w http.ResponseWriter, r *http.Request) error {
	identity, err := middleware.GetIdentityFromContext(r.Context())
	if err != nil {
		return apierror.NewAPIError(http.StatusUnauthorized, err)
	}
	return WriteJSON(w, http.StatusOK, identity)
}

type MessageResponse struct {
	Message string `json:"message" example:"logged out successfully"`
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

// ListActiveSessions godoc
//
//	@Summary		List active sessions
//	@Description	Lists valid sessions for the authenticated identity on the current service surface.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{array}		SessionMetadataResponse
//	@Failure		401	{object}	apierror.APIError
//	@Failure		500	{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/auth/customer/sessions [get]
//	@Router			/auth/merchant/sessions [get]
//	@Router			/auth/admin/sessions [get]
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

// RevokeOtherSessions godoc
//
//	@Summary		Revoke other sessions
//	@Description	Revokes all other valid sessions for the authenticated identity on the current service surface.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	MessageResponse
//	@Failure		401	{object}	apierror.APIError
//	@Failure		500	{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/auth/customer/sessions [delete]
//	@Router			/auth/merchant/sessions [delete]
//	@Router			/auth/admin/sessions [delete]
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

// RevokeSessionByID godoc
//
//	@Summary		Revoke session by ID
//	@Description	Revokes one session belonging to the authenticated identity.
//	@Tags			auth
//	@Produce		json
//	@Param			id	path		string	true	"Session UUID"
//	@Success		200	{object}	MessageResponse
//	@Failure		400	{object}	apierror.APIError
//	@Failure		401	{object}	apierror.APIError
//	@Failure		404	{object}	apierror.APIError
//	@Failure		500	{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/auth/customer/sessions/{id} [delete]
//	@Router			/auth/merchant/sessions/{id} [delete]
//	@Router			/auth/admin/sessions/{id} [delete]
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
