package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/internal/middleware"
	"github.com/skynicklaus/ecommerce-api/internal/validation"
)

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	type mockResponse struct {
		Message string `json:"message"`
	}

	tests := []struct {
		name       string
		statusCode int
		value      any
		wantBody   string
		wantErr    bool
	}{
		{
			name:       "success_struct",
			statusCode: http.StatusOK,
			value:      mockResponse{Message: "hello"},
			wantBody:   `{"message":"hello"}`,
			wantErr:    false,
		},
		{
			name:       "success_map",
			statusCode: http.StatusCreated,
			value:      map[string]string{"foo": "bar"},
			wantBody:   `{"foo":"bar"}`,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			err := WriteJSON(w, tt.statusCode, tt.value)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, "application/json", w.Header().Get("Content-Type"))
				require.Equal(t, tt.statusCode, w.Code)
				require.JSONEq(t, tt.wantBody, w.Body.String())
			}
		})
	}
}

func TestWriteText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		value      string
		wantBody   string
		wantErr    bool
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			value:      "hello world",
			wantBody:   "hello world",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			err := WriteText(w, tt.statusCode, tt.value)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
				require.Equal(t, tt.statusCode, w.Code)
				require.Equal(t, tt.wantBody, w.Body.String())
			}
		})
	}
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()

	type mockRequest struct {
		Name string `json:"name" validate:"required"`
	}

	tests := []struct {
		name         string
		reqBody      string
		wantVal      string
		wantAPIError bool
		wantStatus   int
	}{
		{
			name:         "success",
			reqBody:      `{"name":"john"}`,
			wantVal:      "john",
			wantAPIError: false,
		},
		{
			name:         "invalid_json",
			reqBody:      `{"name":`,
			wantAPIError: true,
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "body_too_large",
			reqBody:      `{"name":"` + strings.Repeat("a", maxRequestBodyBytes+10) + `"}`,
			wantAPIError: true,
			wantStatus:   http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(tt.reqBody))

			var payload mockRequest
			err := decodeJSON(w, req, &payload)

			if tt.wantAPIError {
				require.Error(t, err)
				var apiErr apierror.APIError
				require.ErrorAs(t, err, &apiErr)
				require.Equal(t, tt.wantStatus, apiErr.StatusCode)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantVal, payload.Name)
			}
		})
	}
}

func TestOrganizationFromCtx(t *testing.T) {
	t.Parallel()

	mockOrg := db.Organization{
		Name: "test organization",
	}

	tests := []struct {
		name    string
		ctx     context.Context
		wantOrg db.Organization
		wantErr bool
	}{
		{
			name: "success",
			ctx: context.WithValue(
				context.Background(),
				middleware.OrganizationContextKey{},
				mockOrg,
			),
			wantOrg: mockOrg,
			wantErr: false,
		},
		{
			name:    "failure_no_context_value",
			ctx:     context.Background(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			org, err := organizationFromCtx(tt.ctx)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantOrg, org)
			}
		})
	}
}

func TestV1Handler_Validate(t *testing.T) {
	t.Parallel()

	type mockValidateRequest struct {
		Email string `json:"email" validate:"required,email"`
		Age   int    `json:"age"   validate:"required,gte=18"`
	}

	validator := validation.NewValidator()
	h := &V1Handler{
		validator: validator,
	}

	tests := []struct {
		name    string
		req     any
		wantErr bool
	}{
		{
			name: "success",
			req: mockValidateRequest{
				Email: "john@example.com",
				Age:   25,
			},
			wantErr: false,
		},
		{
			name: "failure_missing_email",
			req: mockValidateRequest{
				Age: 25,
			},
			wantErr: true,
		},
		{
			name: "failure_underage",
			req: mockValidateRequest{
				Email: "john@example.com",
				Age:   17,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.validate(tt.req)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
