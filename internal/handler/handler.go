package handler

import (
	"net/http"

	"github.com/go-playground/validator/v10"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
	"github.com/skynicklaus/ecommerce-api/internal/validation"
	"github.com/skynicklaus/ecommerce-api/util"
)

type Handler interface {
	UserCredentialRegistration(http.ResponseWriter, *http.Request) error
	PlatformUserCredentialRegistration(http.ResponseWriter, *http.Request) error
	CustomerCredentialRegistration(http.ResponseWriter, *http.Request) error
	validate(req any) error
}

type V1Handler struct {
	store     db.Store
	logger    *util.ServerLogger
	validator *validator.Validate
	cache     *cache.RedisClient
}

func NewV1Handler(store db.Store, logger *util.ServerLogger, cache *cache.RedisClient) Handler {
	validator := validation.NewValidator()

	return &V1Handler{
		store:     store,
		logger:    logger,
		validator: validator,
		cache:     cache,
	}
}

func (h *V1Handler) validate(req any) error {
	return h.validator.Struct(req)
}
