package apierror

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/skynicklaus/ecommerce-api/internal/validation"
)

type APIError struct {
	StatusCode int                     `json:"statusCode"`
	Message    string                  `json:"message"`
	Fields     []validation.FieldError `json:"fields,omitempty"`
}

func NewAPIError(statusCode int, err error) APIError {
	message := err.Error()
	if statusCode == http.StatusUnprocessableEntity {
		message = "validation error"
	}

	return APIError{
		StatusCode: statusCode,
		Message:    message,
		Fields:     validation.ParseValidationError(err),
	}
}

func (e APIError) Error() string {
	return e.Message
}

func ErrInvalidJSON() APIError {
	return NewAPIError(http.StatusBadRequest, errors.New("invalid json request"))
}

func ErrValidation(err error) APIError {
	return NewAPIError(http.StatusUnprocessableEntity, err)
}

func Write(w http.ResponseWriter, err APIError) {
	w.WriteHeader(err.StatusCode)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(err)
}
