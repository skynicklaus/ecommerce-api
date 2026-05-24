package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/httplog/v3"
	"github.com/google/uuid"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/util"
)

type (
	IdentityContextKey     struct{}
	SessionTokenContextKey struct{}
)

// IdentityContext holds polymorphic information about the active caller.
type IdentityContext struct {
	IdentityID     uuid.UUID           `json:"identityId"`
	Type           string              `json:"type"`                     // "user" or "customer"
	ActorID        uuid.UUID           `json:"actorId"`                  // User.ID or Customer.ID
	OrganizationID uuid.UUID           `json:"organizationId,omitempty"` // For merchant members
	Name           string              `json:"name"`
	Email          string              `json:"email"`
	Service        util.SessionService `json:"service"`
	SessionID      uuid.UUID           `json:"sessionId"` // Track session ID for current session identification
}

const (
	bearerTokenParts   = 2
	sessionTokenHexLen = 64 // hex encoding of 32 random bytes
	renewalTimeout     = 5 * time.Second
)

// RequireService validates the session token and asserts service group bounds.
//
//nolint:gocognit,funlen // sequential auth gate: format → lookup → service check → identity → renewal
func (m *Middleware) RequireService(
	requiredService util.SessionService,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				apierror.Write(
					w,
					apierror.NewAPIError(
						http.StatusUnauthorized,
						errors.New("missing authorization header"),
					),
				)
				return
			}

			parts := strings.SplitN(authHeader, " ", bearerTokenParts)
			if len(parts) != bearerTokenParts || strings.ToLower(parts[0]) != "bearer" {
				apierror.Write(
					w,
					apierror.NewAPIError(
						http.StatusUnauthorized,
						errors.New("invalid authorization header format"),
					),
				)
				return
			}
			token := strings.TrimSpace(parts[1])
			if len(token) != sessionTokenHexLen {
				apierror.Write(
					w,
					apierror.NewAPIError(
						http.StatusUnauthorized,
						errors.New("invalid token format"),
					),
				)
				return
			}
			tokenHash := util.HashSessionToken(token)

			// Fetch session and identity type in one round-trip. The query filters
			// expired sessions (expires_at > NOW()), so ErrNotFound covers both
			// invalid and expired tokens — clients get the same 401 for both.
			session, dbErr := m.store.GetSessionWithIdentity(ctx, tokenHash)
			if dbErr != nil {
				if errors.Is(dbErr, db.ErrNotFound) {
					apierror.Write(
						w,
						apierror.NewAPIError(
							http.StatusUnauthorized,
							errors.New("invalid or expired session"),
						),
					)
					return
				}

				httplog.SetAttrs(ctx, slog.String("error", dbErr.Error()))
				apierror.Write(
					w,
					apierror.NewAPIError(
						http.StatusInternalServerError,
						errors.New("failed to retrieve session"),
					),
				)
				return
			}

			// Service boundary check: a buyer token must not reach merchant routes.
			if session.Service != string(requiredService) {
				apierror.Write(
					w,
					apierror.NewAPIError(
						http.StatusForbidden,
						errors.New("unauthorized service access for session"),
					),
				)
				return
			}

			// Resolve polymorphic identity based on the joined identity_type.
			// OrganizationID is stored in the session at login time — no extra DB round-trip.
			identityCtx, resolveErr := m.resolveIdentity(
				ctx,
				session.IdentityID,
				session.IdentityType,
				requiredService,
				session.ActiveOrganizationID,
			)
			if resolveErr != nil {
				httplog.SetAttrs(ctx, slog.String("error", resolveErr.Error()))
				apierror.Write(
					w,
					apierror.NewAPIError(
						http.StatusUnauthorized,
						errors.New("failed to resolve session identity"),
					),
				)
				return
			}

			responseExpiry := session.ExpiresAt
			// Slide expiry if less than half the session lifetime remains, but never
			// past the absolute ceiling (created_at + service max). Once the ceiling
			// is reached the session expires naturally on its next request.
			if time.Until(session.ExpiresAt) < util.SessionTTL/2 {
				absoluteCeiling := session.CreatedAt.Add(
					util.AbsoluteSessionMax(util.SessionService(session.Service)),
				)
				if time.Now().Before(absoluteCeiling) {
					projected := time.Now().Add(util.SessionTTL)
					if projected.After(absoluteCeiling) {
						projected = absoluteCeiling
					}
					responseExpiry = projected
					go func() {
						renewCtx, cancel := context.WithTimeout(
							context.WithoutCancel(ctx),
							renewalTimeout,
						)
						defer cancel()
						_, _, _ = m.renewalG.Do(tokenHash, func() (any, error) {
							return nil, m.store.RenewSession(renewCtx, db.RenewSessionParams{
								Token:     tokenHash,
								ExpiresAt: projected,
							})
						})
					}()
				}
			}
			w.Header().Set("X-Session-Expires-At", responseExpiry.UTC().Format(time.RFC3339))

			identityCtx.SessionID = session.SessionID

			ctx = context.WithValue(ctx, IdentityContextKey{}, identityCtx)
			ctx = context.WithValue(ctx, SessionTokenContextKey{}, tokenHash)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (m *Middleware) resolveIdentity(
	ctx context.Context,
	identityID uuid.UUID,
	identityType string,
	service util.SessionService,
	orgID *uuid.UUID,
) (IdentityContext, error) {
	resolvedOrgID := uuid.Nil
	if orgID != nil {
		resolvedOrgID = *orgID
	}

	switch identityType {
	case string(util.IdentityCustomer):
		customer, custErr := m.store.GetCustomerByIdentityID(ctx, identityID)
		if custErr != nil {
			return IdentityContext{}, custErr
		}
		return IdentityContext{
			IdentityID:     identityID,
			Type:           string(util.IdentityCustomer),
			ActorID:        customer.ID,
			OrganizationID: resolvedOrgID,
			Name:           customer.Name,
			Email:          customer.Email,
			Service:        service,
			SessionID:      uuid.Nil, // set by RequireService after resolveIdentity returns
		}, nil

	case string(util.IdentityUser):
		user, userErr := m.store.GetUserByIdentityID(ctx, identityID)
		if userErr != nil {
			return IdentityContext{}, userErr
		}
		return IdentityContext{
			IdentityID:     identityID,
			Type:           string(util.IdentityUser),
			ActorID:        user.ID,
			OrganizationID: resolvedOrgID,
			Name:           user.Name,
			Email:          user.Email,
			Service:        service,
			SessionID:      uuid.Nil, // set by RequireService after resolveIdentity returns
		}, nil

	default:
		return IdentityContext{}, fmt.Errorf("unknown identity type %q in session", identityType)
	}
}

func GetIdentityFromContext(ctx context.Context) (IdentityContext, error) {
	val := ctx.Value(IdentityContextKey{})
	if val == nil {
		return IdentityContext{}, errors.New("no identity context available")
	}
	identity, ok := val.(IdentityContext)
	if !ok {
		return IdentityContext{}, errors.New("corrupt identity context")
	}
	return identity, nil
}

func GetTokenFromContext(ctx context.Context) (string, error) {
	val := ctx.Value(SessionTokenContextKey{})
	if val == nil {
		return "", errors.New("no session token available")
	}
	token, ok := val.(string)
	if !ok {
		return "", errors.New("corrupt session token context")
	}
	return token, nil
}
