package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"github.com/go-chi/traceid"

	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/internal/handler"
	midware "github.com/skynicklaus/ecommerce-api/internal/middleware"
	"github.com/skynicklaus/ecommerce-api/util"
)

func (s *Server) RegisterRoutes() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.StripSlashes)
	r.Use(traceid.Middleware)
	//nolint:exhaustruct // too many fields
	r.Use(httplog.RequestLogger(s.logger.Logger, &httplog.Options{
		Schema:             s.logger.LogFormat,
		RecoverPanics:      true,
		LogRequestHeaders:  []string{"Origin"},
		LogResponseHeaders: []string{},
		LogRequestBody:     isDebugBodyLoggingAllowed,
		LogResponseBody:    isDebugBodyLoggingAllowed,
		LogExtraAttrs: func(req *http.Request, reqBody string, respStatus int) []slog.Attr {
			if respStatus == 400 || respStatus == 422 {
				req.Header.Del("Authorization")
				return []slog.Attr{slog.String("curl", httplog.CURL(req, reqBody))}
			}
			return nil
		},
	}))

	midware := midware.New(s.store)

	r.Use(midware.SecurityHeaders)

	v1Handler := handler.NewV1Handler(s.store, s.logger, s.redis, s.storage)

	r.Get("/health", s.make(func(w http.ResponseWriter, _ *http.Request) error {
		return handler.WriteText(w, http.StatusOK, "OK")
	}))

	r.Route("/v1", func(r chi.Router) {
		r.Post("/users/merchant", s.make(v1Handler.UserCredentialRegistration))
		r.Post("/customer", s.make(v1Handler.CustomerCredentialRegistration))

		// Open login routes.
		r.Post("/auth/customer/login", s.make(v1Handler.LoginCustomer))
		r.Post("/auth/merchant/login", s.make(v1Handler.LoginMerchant))
		r.Post("/auth/admin/login", s.make(v1Handler.LoginAdmin))

		// Storefront (buyer-facing) reads — public, cross-tenant, active products only.
		r.Get("/products", s.make(v1Handler.ListProducts))
		r.Get("/products/{slug_or_id}", s.make(v1Handler.GetProductDetails))

		// Protected Customer Routes
		r.Group(func(r chi.Router) {
			r.Use(midware.RequireService(util.SessionServiceBuyerPlatform))

			r.Post("/auth/customer/logout", s.make(v1Handler.Logout))
			r.Get("/auth/customer/me", s.make(v1Handler.GetMe))
			r.Get("/auth/customer/sessions", s.make(v1Handler.ListActiveSessions))
			r.Delete("/auth/customer/sessions", s.make(v1Handler.RevokeOtherSessions))
			r.Delete("/auth/customer/sessions/{id}", s.make(v1Handler.RevokeSessionByID))
		})

		// Protected Merchant Routes
		r.Group(func(r chi.Router) {
			r.Use(midware.RequireService(util.SessionServiceMerchantPanel))
			r.Use(midware.ValidateOrganization)

			r.Post("/auth/merchant/logout", s.make(v1Handler.Logout))
			r.Get("/auth/merchant/me", s.make(v1Handler.GetMe))
			r.Get("/auth/merchant/sessions", s.make(v1Handler.ListActiveSessions))
			r.Delete("/auth/merchant/sessions", s.make(v1Handler.RevokeOtherSessions))
			r.Delete("/auth/merchant/sessions/{id}", s.make(v1Handler.RevokeSessionByID))

			r.Post("/product-assets", s.make(v1Handler.PreUploadAssets))
			r.Post("/products", s.make(v1Handler.CreateProduct))
			r.Patch("/products/{id}/status", s.make(v1Handler.UpdateProductStatus))
		})

		// Protected Platform Admin Routes
		r.Group(func(r chi.Router) {
			r.Use(midware.RequireService(util.SessionServiceAdminPanel))

			r.Post("/auth/admin/logout", s.make(v1Handler.Logout))
			r.Get("/auth/admin/me", s.make(v1Handler.GetMe))
			r.Get("/auth/admin/sessions", s.make(v1Handler.ListActiveSessions))
			r.Delete("/auth/admin/sessions", s.make(v1Handler.RevokeOtherSessions))
			r.Delete("/auth/admin/sessions/{id}", s.make(v1Handler.RevokeSessionByID))

			// Platform admin creation is authenticated — first admin is bootstrapped
			// via the migrate command using PLATFORM_ADMIN_EMAIL / PLATFORM_ADMIN_PASSWORD.
			r.Post("/users/platform", s.make(v1Handler.PlatformUserCredentialRegistration))
		})
	})

	return r
}

type APIFunc func(http.ResponseWriter, *http.Request) error

func (s *Server) make(h APIFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			httplog.SetAttrs(
				r.Context(),
				slog.String("error", err.Error()),
			)
			s.handleError(w, err)
		}
	}
}

func (s *Server) handleError(w http.ResponseWriter, err error) {
	if apiErr, ok := errors.AsType[apierror.APIError](err); ok {
		if writeErr := handler.WriteJSON(w, apiErr.StatusCode, apiErr); writeErr != nil {
			s.logger.Error("failed to write response", slog.Any("err", writeErr))
		}
		return
	}

	errResp := map[string]any{
		"statusCode": http.StatusInternalServerError,
		"message":    "an unexpected error occurred",
	}
	if writeErr := handler.WriteJSON(w, http.StatusInternalServerError, errResp); writeErr != nil {
		s.logger.Error("failed to write response", slog.Any("err", writeErr))
	}
}

func isDebugHeaderSet(r *http.Request) bool {
	return r.Header.Get("Debug") == "reveal-body-logs"
}

// isDebugBodyLoggingAllowed enables request body logging for non-auth routes only.
// Auth routes are excluded to prevent passwords from appearing in logs regardless
// of the Debug header value.
func isDebugBodyLoggingAllowed(r *http.Request) bool {
	return isDebugHeaderSet(r) && !strings.HasPrefix(r.URL.Path, "/v1/auth/")
}
