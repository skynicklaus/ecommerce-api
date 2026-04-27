package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/middleware"
)

func WriteJSON(w http.ResponseWriter, statusCode int, v any) error {
	w.WriteHeader(statusCode)
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(v)
}

func WriteText(w http.ResponseWriter, statusCode int, v string) error {
	w.WriteHeader(statusCode)
	_, err := w.Write([]byte(v))
	return err
}

func decodeJSON[T any](r *http.Request, req *T) error {
	return json.NewDecoder(r.Body).Decode(req)
}

func organizationFromCtx(ctx context.Context) (db.Organization, error) {
	organization, ok := ctx.Value(middleware.OrganizationContextKey{}).(db.Organization)
	if !ok {
		return db.Organization{}, errors.New("invalid organization")
	}

	return organization, nil
}
