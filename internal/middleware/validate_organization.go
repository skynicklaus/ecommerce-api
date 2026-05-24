package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/httplog/v3"
	"github.com/google/uuid"

	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/util"
)

type OrganizationContextKey struct{}

func (m *Middleware) ValidateOrganization(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		identity, err := GetIdentityFromContext(ctx)
		if err != nil {
			apierror.Write(w, apierror.NewAPIError(http.StatusUnauthorized, errors.New("authentication required")))
			return
		}

		if identity.OrganizationID == uuid.Nil {
			apierror.Write(
				w,
				apierror.NewAPIError(http.StatusForbidden, errors.New("no organization associated with this account")),
			)
			return
		}

		organization, dbErr := m.store.GetOrganizationByID(ctx, identity.OrganizationID)
		if dbErr != nil {
			httplog.SetAttrs(ctx, slog.String("error", dbErr.Error()))
			apierror.Write(
				w,
				apierror.NewAPIError(http.StatusInternalServerError, errors.New("failed to resolve organization")),
			)
			return
		}

		if organization.Status != string(util.OrganizationStatusActive) {
			apierror.Write(
				w,
				apierror.NewAPIError(http.StatusForbidden, errors.New("organization is not active")),
			)
			return
		}

		ctx = context.WithValue(ctx, OrganizationContextKey{}, organization)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
