package server

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"github.com/go-chi/traceid"
	httpSwagger "github.com/swaggo/http-swagger"

	docs "github.com/skynicklaus/ecommerce-api/docs"
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
	debugBodyLogging := isDebugBodyLoggingAllowed()
	//nolint:exhaustruct // too many fields
	r.Use(httplog.RequestLogger(s.logger.Logger, &httplog.Options{
		Schema:             s.logger.LogFormat,
		RecoverPanics:      true,
		LogRequestHeaders:  []string{"Origin"},
		LogResponseHeaders: []string{},
		LogRequestBody:     debugBodyLogging,
		LogResponseBody:    debugBodyLogging,
		LogExtraAttrs: func(req *http.Request, reqBody string, respStatus int) []slog.Attr {
			if respStatus == 400 || respStatus == 422 {
				req.Header.Del("Authorization")
				return []slog.Attr{slog.String("curl", httplog.CURL(req, reqBody))}
			}
			return nil
		},
	}))

	midware := midware.New(s.store, s.redis)

	r.Use(midware.SecurityHeaders)

	v1Handler := handler.NewV1Handler(s.store, s.logger, s.redis, s.storage)

	r.Get("/health", s.make(func(w http.ResponseWriter, _ *http.Request) error {
		return handler.WriteText(w, http.StatusOK, "OK")
	}))
	if isSwaggerEnabled() {
		configureSwaggerInfo()
		r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json")))
	}

	r.Route("/v1", func(r chi.Router) {
		r.Post("/customer", s.make(v1Handler.CustomerCredentialRegistration))

		// Open login routes.
		r.Post("/auth/customer/login", s.make(v1Handler.LoginCustomer))
		r.Post("/auth/merchant/login", s.make(v1Handler.LoginMerchant))
		r.Post("/auth/admin/login", s.make(v1Handler.LoginAdmin))

		// Storefront (buyer-facing) reads — public, cross-tenant, active products only.
		r.Get("/products", s.make(v1Handler.ListActiveProducts))
		r.Get("/products/{org_id}/{slug_or_id}", s.make(v1Handler.GetActiveProductDetails))

		// Protected Customer Routes
		r.Group(func(r chi.Router) {
			r.Use(midware.RequireService(util.SessionServiceBuyerPlatform))

			r.Post("/auth/customer/logout", s.make(v1Handler.Logout))
			r.Get("/auth/customer/me", s.make(v1Handler.GetMe))
			r.Get("/auth/customer/sessions", s.make(v1Handler.ListActiveSessions))
			r.Delete("/auth/customer/sessions", s.make(v1Handler.RevokeOtherSessions))
			r.Delete("/auth/customer/sessions/{id}", s.make(v1Handler.RevokeSessionByID))
		})

		// Protected Merchant account/session routes.
		// These require a merchant-panel session but do not require an active organization,
		// so pending merchants can still inspect their account state and manage sessions.
		r.Group(func(r chi.Router) {
			r.Use(midware.RequireService(util.SessionServiceMerchantPanel))

			r.Post("/auth/merchant/logout", s.make(v1Handler.Logout))
			r.Get("/auth/merchant/me", s.make(v1Handler.GetMe))
			r.Get("/auth/merchant/sessions", s.make(v1Handler.ListActiveSessions))
			r.Delete("/auth/merchant/sessions", s.make(v1Handler.RevokeOtherSessions))
			r.Delete("/auth/merchant/sessions/{id}", s.make(v1Handler.RevokeSessionByID))
		})

		// Protected Merchant operational routes.
		// These require both a merchant-panel session and an active organization.
		r.Group(func(r chi.Router) {
			r.Use(midware.RequireService(util.SessionServiceMerchantPanel))
			r.Use(midware.ValidateOrganization)

			r.Post("/product-assets", s.make(v1Handler.PreUploadAssets))
			r.Post("/products", s.make(v1Handler.CreateProduct))
			r.Patch("/products/{id}/status", s.make(v1Handler.UpdateProductStatus))
			r.Put("/products/{id}", s.make(v1Handler.UpdateProduct))
			r.Delete("/products/{id}", s.make(v1Handler.DeleteProduct))
			r.Get("/merchant/products", s.make(v1Handler.ListMerchantProducts))
			r.Get("/merchant/products/{id}", s.make(v1Handler.GetMerchantProductDetails))
			r.Get("/merchant/products/{id}/inventory", s.make(v1Handler.ListProductInventory))
			r.Post("/merchant/warehouses", s.make(v1Handler.CreateWarehouse))
			r.Get("/merchant/warehouses", s.make(v1Handler.ListWarehouses))
			r.Put("/merchant/warehouses/{id}", s.make(v1Handler.UpdateWarehouse))
			r.Put("/merchant/inventory", s.make(v1Handler.UpsertInventory))
			r.Get("/merchant/inventory", s.make(v1Handler.ListInventory))
		})

		// Protected Platform Admin Routes
		r.Group(func(r chi.Router) {
			r.Use(midware.RequireService(util.SessionServiceAdminPanel))

			r.Post("/auth/admin/logout", s.make(v1Handler.Logout))
			r.Get("/auth/admin/me", s.make(v1Handler.GetMe))
			r.Get("/auth/admin/sessions", s.make(v1Handler.ListActiveSessions))
			r.Delete("/auth/admin/sessions", s.make(v1Handler.RevokeOtherSessions))
			r.Delete("/auth/admin/sessions/{id}", s.make(v1Handler.RevokeSessionByID))

			// User creation is admin-gated — first admin is bootstrapped via migrate.
			r.Post("/users/platform", s.make(v1Handler.PlatformUserCredentialRegistration))
			r.Post("/users/merchant", s.make(v1Handler.UserCredentialRegistration))
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

// isDebugBodyLoggingAllowed returns a per-request predicate when DEBUG_BODY_LOGGING=true,
// or nil (disables body logging entirely) otherwise. Called once at startup so os.Getenv
// is not hit on every request.
func isDebugBodyLoggingAllowed() func(*http.Request) bool {
	if os.Getenv("DEBUG_BODY_LOGGING") != "true" {
		return nil
	}
	return func(r *http.Request) bool {
		return r.Header.Get("Debug") == "reveal-body-logs" && !isSensitiveRoute(r.URL.Path)
	}
}

// isSensitiveRoute returns true for any path whose request body may contain credentials.
func isSensitiveRoute(path string) bool {
	return strings.HasPrefix(path, "/v1/auth/") ||
		strings.HasPrefix(path, "/v1/users/") ||
		path == "/v1/customer"
}

func isSwaggerEnabled() bool {
	switch strings.ToLower(os.Getenv("APP_ENV")) {
	case "local", "development", "staging":
		return true
	default:
		return false
	}
}

func configureSwaggerInfo() {
	externalBaseURL := strings.TrimSpace(os.Getenv("EXTERNAL_BASE_URL"))
	if externalBaseURL == "" {
		docs.SwaggerInfo.Host = "localhost:8080"
		return
	}

	parsed, err := url.Parse(externalBaseURL)
	if err != nil || parsed.Host == "" {
		docs.SwaggerInfo.Host = strings.TrimSuffix(externalBaseURL, "/")
		return
	}

	docs.SwaggerInfo.Host = parsed.Host
	if parsed.Scheme != "" {
		docs.SwaggerInfo.Schemes = []string{parsed.Scheme}
	}
}
