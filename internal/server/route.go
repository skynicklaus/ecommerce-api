package server

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"github.com/go-chi/traceid"

	"github.com/skynicklaus/ecommerce-api/internal/handler"
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
		LogRequestBody:     isDebugHeaderSet,
		LogResponseBody:    isDebugHeaderSet,
		LogExtraAttrs: func(req *http.Request, reqBody string, respStatus int) []slog.Attr {
			if respStatus == 400 || respStatus == 422 {
				req.Header.Del("Authorization")
				return []slog.Attr{slog.String("curl", httplog.CURL(req, reqBody))}
			}
			return nil
		},
	}))

	v1Handler := handler.NewV1Handler(s.store, s.logger, s.redis)

	r.Get("/health", s.make(func(w http.ResponseWriter, _ *http.Request) error {
		return handler.WriteText(w, http.StatusOK, "OK")
	}))

	r.Route("/v1", func(r chi.Router) {
		r.Post("/users/platform", s.make(v1Handler.PlatformUserCredentialRegistration))
		r.Post("/users/merchant", s.make(v1Handler.UserCredentialRegistration))
		r.Post("/customer", s.make(v1Handler.CustomerCredentialRegistration))
	})

	return r
}

type APIFunc func(http.ResponseWriter, *http.Request) error

func (s *Server) make(h APIFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			httplog.SetAttrs(r.Context(),
				slog.String("error", err.Error()),
			)
			s.handleError(w, err)
		}
	}
}

func (s *Server) handleError(w http.ResponseWriter, err error) {
	if apiErr, ok := errors.AsType[handler.APIError](err); ok {
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
