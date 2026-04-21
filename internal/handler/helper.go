package handler

import (
	"encoding/json"
	"net/http"

	"github.com/skynicklaus/ecommerce-api/internal/validation"
)

type APIError struct {
	StatusCode int                     `json:"statusCode"`
	Message    string                  `json:"message"`
	Fields     []validation.FieldError `json:"fields,omitempty"`
}

func NewAPIError(statusCode int, message string, err error) APIError {
	return APIError{
		StatusCode: statusCode,
		Message:    err.Error(),
		Fields:     validation.ParseValidationError(err),
	}
}

func (e APIError) Error() string {
	return e.Message
}

func errInvalidJSON() APIError {
	return NewAPIError(http.StatusBadRequest, "invalid json request", nil)
}

func errValidation(err error) APIError {
	return NewAPIError(http.StatusUnprocessableEntity, "validation error", err)
}

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
