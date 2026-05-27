package middleware

import (
	"context"
	"errors"
	"net/http"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/util"
)

// ApplyRLSContext prepares database RLS settings for downstream queries.
func (m *Middleware) ApplyRLSContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		identity, err := GetIdentityFromContext(ctx)
		if err != nil {
			apierror.Write(w, apierror.NewAPIError(http.StatusUnauthorized, errors.New("authentication required")))
			return
		}

		organization, err := organizationFromContext(ctx)
		if err != nil {
			apierror.Write(
				w,
				apierror.NewAPIError(http.StatusInternalServerError, errors.New("organization context unavailable")),
			)
			return
		}

		if organization.ID != identity.OrganizationID {
			apierror.Write(
				w,
				apierror.NewAPIError(http.StatusForbidden, errors.New("organization context does not match identity")),
			)
			return
		}

		ctx = db.WithRLSContext(ctx, db.RLSContext{
			IdentityID:     identity.IdentityID,
			OrganizationID: organization.ID,
			// TODO: Derive platform user/admin capabilities from resolved roles
			// once platform roles become more granular than admin-panel access.
			IsPlatformUser:  identity.Service == util.SessionServiceAdminPanel,
			IsPlatformAdmin: identity.Service == util.SessionServiceAdminPanel,
		})

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func organizationFromContext(ctx context.Context) (db.Organization, error) {
	organization, ok := ctx.Value(OrganizationContextKey{}).(db.Organization)
	if !ok {
		return db.Organization{}, errors.New("no organization context available")
	}
	return organization, nil
}
