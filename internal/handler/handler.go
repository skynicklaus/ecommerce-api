package handler

import (
	"net/http"
	"os"

	"github.com/go-playground/validator/v10"
	"golang.org/x/sync/singleflight"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/allowed"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
	"github.com/skynicklaus/ecommerce-api/internal/storage"
	"github.com/skynicklaus/ecommerce-api/internal/validation"
	"github.com/skynicklaus/ecommerce-api/util"
)

type Handler interface {
	// Organization
	CreateOrganization(http.ResponseWriter, *http.Request) error

	// Product — storefront (public)
	GetActiveProductDetails(http.ResponseWriter, *http.Request) error
	ListActiveProducts(http.ResponseWriter, *http.Request) error

	// Product — merchant catalog (authenticated)
	CreateProduct(http.ResponseWriter, *http.Request) error
	UpdateProductStatus(http.ResponseWriter, *http.Request) error
	UpdateProduct(http.ResponseWriter, *http.Request) error
	DeleteProduct(http.ResponseWriter, *http.Request) error
	ListMerchantProducts(http.ResponseWriter, *http.Request) error
	GetMerchantProductDetails(http.ResponseWriter, *http.Request) error

	// Product Asset
	PreUploadAssets(http.ResponseWriter, *http.Request) error

	// Registration
	UserCredentialRegistration(http.ResponseWriter, *http.Request) error
	PlatformUserCredentialRegistration(http.ResponseWriter, *http.Request) error
	CustomerCredentialRegistration(http.ResponseWriter, *http.Request) error

	// Auth
	LoginCustomer(http.ResponseWriter, *http.Request) error
	LoginMerchant(http.ResponseWriter, *http.Request) error
	LoginAdmin(http.ResponseWriter, *http.Request) error
	Logout(http.ResponseWriter, *http.Request) error
	GetMe(http.ResponseWriter, *http.Request) error

	// Sessions
	ListActiveSessions(http.ResponseWriter, *http.Request) error
	RevokeOtherSessions(http.ResponseWriter, *http.Request) error
	RevokeSessionByID(http.ResponseWriter, *http.Request) error

	validate(req any) error
}

type V1Handler struct {
	store         db.Store
	logger        *util.ServerLogger
	validator     *validator.Validate
	cache         *cache.Client
	storage       *storage.S3Storage
	mime          *allowed.MimeList
	storageRegion *string
	bucket        *string
	maxImageSize  int64
	maxVideoSize  int64
	presignG      singleflight.Group
}

func NewV1Handler(
	store db.Store,
	logger *util.ServerLogger,
	cache *cache.Client,
	storage *storage.S3Storage,
) Handler {
	storageRegion := os.Getenv("AWS_REGION")
	bucket := os.Getenv("S3_BUCKET")
	validator := validation.NewValidator()

	const maxImageSize = 10 * 1024 * 1024
	const maxVideoSize = 120 * 1024 * 1024
	config := allowed.Config{
		AllowImages:    true,
		AllowVideos:    true,
		AllowDocuments: false,
		MaxImageSize:   maxImageSize,
		MaxVideoSize:   maxVideoSize,
		CustomTypes:    nil,
	}

	return &V1Handler{
		store:         store,
		logger:        logger,
		validator:     validator,
		cache:         cache,
		storage:       storage,
		mime:          allowed.LoadFromConfig(config),
		storageRegion: &storageRegion,
		bucket:        &bucket,
		maxImageSize:  config.MaxImageSize,
		maxVideoSize:  config.MaxVideoSize,
		presignG:      singleflight.Group{},
	}
}

func (h *V1Handler) validate(req any) error {
	return h.validator.Struct(req)
}
