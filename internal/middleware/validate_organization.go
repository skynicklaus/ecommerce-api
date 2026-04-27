package middleware

import (
	"context"
	"net/http"
	"os"

	"github.com/google/uuid"

	"github.com/skynicklaus/ecommerce-api/internal/apierror"
)

type OrganizationContextKey struct{}

func (m *Middleware) VaslidateOrganization(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uuidString := os.Getenv("ORGANIZATION_ID")

		organizationID, err := uuid.Parse(uuidString)
		if err != nil {
			apiErr := apierror.NewAPIError(http.StatusBadRequest, err)
			apierror.Write(w, apiErr)
			return
		}

		organization, err := m.store.GetOrganizationByID(r.Context(), organizationID)
		if err != nil {
			apiErr := apierror.NewAPIError(http.StatusBadRequest, err)
			apierror.Write(w, apiErr)
			return
		}

		ctx := context.WithValue(r.Context(), OrganizationContextKey{}, organization)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
