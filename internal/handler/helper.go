package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/internal/middleware"
)

func WriteJSON(w http.ResponseWriter, statusCode int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(v)
}

func WriteText(w http.ResponseWriter, statusCode int, v string) error {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(statusCode)
	_, err := w.Write([]byte(v))
	return err
}

const maxRequestBodyBytes = 1 * 1024 * 1024 // 1 MB

func decodeJSON[T any](w http.ResponseWriter, r *http.Request, req *T) error {
	// Restrict request body size to prevent memory exhaustion.
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return apierror.NewAPIError(
				http.StatusRequestEntityTooLarge,
				errors.New("request body too large"),
			)
		}

		return apierror.ErrInvalidJSON()
	}
	return nil
}

func organizationFromCtx(ctx context.Context) (db.Organization, error) {
	organization, ok := ctx.Value(middleware.OrganizationContextKey{}).(db.Organization)
	if !ok {
		return db.Organization{}, errors.New("invalid organization")
	}

	return organization, nil
}
