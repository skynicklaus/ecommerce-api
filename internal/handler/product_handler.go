package handler

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/errgroup"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/util"
)

type ProductAssetRequest struct {
	Token     string  `json:"token"             validate:"required"`
	IsPrimary bool    `json:"isPrimary"`
	AltText   *string `json:"altText,omitempty"`
	SortOrder int16   `json:"sortOrder"`
}

type VariantRequest struct {
	Sku               string               `json:"sku"               validate:"required"`
	Name              string               `json:"name"              validate:"required"`
	Price             decimal.Decimal      `json:"price"             validate:"required"`
	AttributeValueIDs []int64              `json:"attributeValueIds" validate:"required"`
	Asset             *ProductAssetRequest `json:"asset,omitempty"`
}

type CreateProductRequest struct {
	Name          string                `json:"name"          validate:"required"`
	Slug          string                `json:"slug"          validate:"required"`
	CategoryID    uuid.UUID             `json:"categoryId"    validate:"required"`
	Description   json.RawMessage       `json:"description"   validate:"required"            swaggertype:"object"`
	Specification json.RawMessage       `json:"specification" validate:"required"            swaggertype:"object"`
	Assets        []ProductAssetRequest `json:"assets"`
	Variants      []VariantRequest      `json:"variants"      validate:"required,min=1,dive"`
}

// CreateProduct godoc
//
//	@Summary		Create product
//	@Description	Creates a draft product with variants and previously pre-uploaded assets. Supports Idempotency-Key for safe retries.
//	@Tags			products
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string					false	"Optional idempotency key for safe retries"
//	@Param			request			body		CreateProductRequest	true	"Product payload"
//	@Success		201				{object}	db.CreateProductTxResults
//	@Success		200				{object}	db.CreateProductTxResults	"Idempotent replay result"
//	@Failure		400				{object}	apierror.APIError
//	@Failure		401				{object}	apierror.APIError
//	@Failure		403				{object}	apierror.APIError
//	@Failure		409				{object}	apierror.APIError
//	@Failure		413				{object}	apierror.APIError
//	@Failure		422				{object}	apierror.APIError
//	@Failure		500				{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/products [post]
func (h *V1Handler) CreateProduct( //nolint:gocognit,funlen,gocyclo,cyclop // idempotency + S3 copies + DB tx
	w http.ResponseWriter,
	r *http.Request,
) error {
	ctx := r.Context()
	var committed bool
	organization, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	// Read the body fully so we can hash the exact bytes the client sent.
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		return apierror.NewAPIError(
			http.StatusRequestEntityTooLarge,
			errors.New("request body too large"),
		)
	}

	var req CreateProductRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return apierror.ErrInvalidJSON()
	}
	if err := h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	var (
		requestHash string
		idemKeyPtr  *string
	)
	if idempotencyKey != "" { //nolint:nestif // sequential fast-path checks (cache→DB→lock) before the write path
		sum := sha256.Sum256(body)
		requestHash = hex.EncodeToString(sum[:])
		idemKeyPtr = &idempotencyKey

		// Fast path: cached result.
		if cached, hit, hashErr := h.lookupIdempotentResultFromCache(
			ctx, organization.ID, idempotencyKey, requestHash,
		); hashErr != nil {
			return hashErr
		} else if hit {
			return WriteJSON(w, http.StatusOK, cached)
		}

		// Durability path: DB-persisted key (survives Redis eviction / restart).
		if existing, hit, dbErr := h.lookupIdempotentResultFromDB(
			ctx, organization.ID, idempotencyKey,
		); dbErr != nil {
			return dbErr
		} else if hit {
			h.cacheIdempotentResult(ctx, organization.ID, idempotencyKey, requestHash, existing)
			return WriteJSON(w, http.StatusOK, existing)
		}

		// Acquire lock to block concurrent dupes (the DB unique index is the backstop).
		ok, lockErr := h.cache.AcquireIdempotencyLock(
			ctx, organization.ID, idempotencyKey, idempotencyLockTTL,
		)
		if lockErr != nil {
			return fmt.Errorf("failed to acquire idempotency lock: %w", lockErr)
		}
		if !ok {
			return apierror.NewAPIError(
				http.StatusConflict,
				fmt.Errorf("request with idempotency key %s is already in-flight", idempotencyKey),
			)
		}
		defer func() {
			if !committed {
				_ = h.cache.ReleaseIdempotencyLock(ctx, organization.ID, idempotencyKey)
			}
		}()
	}

	tokens, err := collectAssetTokens(&req)
	if err != nil {
		return err
	}

	// 1. Resolve each token from Redis and verify org ownership.
	resolvedAssets := make(map[string]PendingUpload, len(tokens))
	for _, token := range tokens {
		pending, resolveErr := h.resolvePendingUpload(ctx, token, organization.ID)
		if resolveErr != nil {
			return resolveErr
		}
		resolvedAssets[token] = pending
	}

	// 2. Copy temp objects to final locations in parallel; track for rollback.
	var (
		copiedMu   sync.Mutex
		copiedKeys = make([]string, 0, len(resolvedAssets))
	)

	defer func() {
		if committed {
			return
		}
		copiedMu.Lock()
		keys := append([]string(nil), copiedKeys...)
		copiedMu.Unlock()
		if len(keys) == 0 {
			return
		}
		//nolint:mnd // fixed timeout
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		for _, destKey := range keys {
			if delErr := h.storage.DeleteObject(cleanupCtx, *h.bucket, destKey); delErr != nil {
				h.logger.WarnContext(
					cleanupCtx, "failed to roll back final S3 object",
					slog.String("key", destKey),
					slog.Any("err", delErr),
				)
			}
		}
	}()

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(productAssetCopyConcurrency)
	for _, pending := range resolvedAssets {
		g.Go(func() error {
			if copyErr := h.storage.CopyObject(
				gCtx,
				*h.bucket,
				pending.TempKey,
				pending.FinalKey,
			); copyErr != nil {
				return fmt.Errorf(
					"failed to copy %s to final location: %w",
					pending.TempKey,
					copyErr,
				)
			}
			copiedMu.Lock()
			copiedKeys = append(copiedKeys, pending.FinalKey)
			copiedMu.Unlock()
			return nil
		})
	}
	if waitErr := g.Wait(); waitErr != nil {
		return waitErr
	}

	// 3. Build transaction params.
	txAssets := make([]db.ProductAssetParams, len(req.Assets))
	for i, assetReq := range req.Assets {
		txAssets[i] = buildAssetParams(assetReq, resolvedAssets[assetReq.Token])
	}

	txVariants := make([]db.ProductVariantParams, len(req.Variants))
	for i, variantReq := range req.Variants {
		var txAsset *db.ProductAssetParams
		if variantReq.Asset != nil {
			params := buildAssetParams(*variantReq.Asset, resolvedAssets[variantReq.Asset.Token])
			txAsset = &params
		}
		txVariants[i] = db.ProductVariantParams{
			Sku:               variantReq.Sku,
			Name:              variantReq.Name,
			Price:             variantReq.Price,
			AttributeValueIDs: variantReq.AttributeValueIDs,
			Asset:             txAsset,
		}
	}

	txParams := db.CreateProductTxParams{
		OrganizationID: organization.ID,
		CategoryID:     req.CategoryID,
		Name:           req.Name,
		Slug:           req.Slug,
		Description:    req.Description,
		Specification:  req.Specification,
		IdempotencyKey: idemKeyPtr,
		Variants:       txVariants,
		Assets:         txAssets,
	}

	// 4. Run atomic creation transaction. Failure triggers deferred S3 rollback.
	result, err := h.store.CreateProductTx(ctx, txParams)
	if err != nil { //nolint:nestif // unique-violation fallback requires nested lookup after a raced commit
		// Last-resort idempotency: another worker beat us past the Redis lock and persisted first.
		// The partial UNIQUE index on (organization_id, idempotency_key) fires here.
		if idempotencyKey != "" && db.ConstraintName(err) == "uq_products_org_idem_key" {
			existing, found, dbErr := h.lookupIdempotentResultFromDB(
				ctx, organization.ID, idempotencyKey,
			)
			if dbErr != nil {
				return dbErr
			}
			if found {
				// The S3 copies we made wrote to the same content-hashed final keys as the
				// winning worker's copies — byte-identical content, idempotent S3 PUTs — so
				// skip the rollback defer rather than deleting keys the persisted product depends on.
				committed = true
				h.cacheIdempotentResult(ctx, organization.ID, idempotencyKey, requestHash, existing)
				_ = h.cache.ReleaseIdempotencyLock(ctx, organization.ID, idempotencyKey)
				return WriteJSON(w, http.StatusOK, existing)
			}
		}
		if apiErr, ok := productTxAPIError(err); ok {
			return apiErr
		}
		return fmt.Errorf("failed to save product details: %w", err)
	}
	committed = true

	if idempotencyKey != "" {
		h.cacheIdempotentResult(ctx, organization.ID, idempotencyKey, requestHash, result)
		_ = h.cache.ReleaseIdempotencyLock(ctx, organization.ID, idempotencyKey)
	}

	// 5. Post-success cleanup. WithoutCancel detaches from request cancellation but
	// preserves trace IDs and other request-scoped values so logs stay correlated.
	for _, pending := range resolvedAssets {
		go h.cleanupCommittedUpload(ctx, pending.TempKey, pending.Token)
	}

	return WriteJSON(w, http.StatusCreated, result)
}

func productTxAPIError(err error) (apierror.APIError, bool) {
	switch db.ConstraintName(err) {
	case "uq_products_organization_slug":
		return apierror.NewAPIError(
			http.StatusConflict,
			errors.New("product slug already exists"),
		), true
	case "uq_product_variants_organization_sku":
		return apierror.NewAPIError(
			http.StatusConflict,
			errors.New("product variant sku already exists"),
		), true
	case "uq_product_assets_primary":
		return apierror.NewAPIError(
			http.StatusUnprocessableEntity,
			errors.New("only one primary product asset is allowed"),
		), true
	case "uq_product_assets_variant":
		return apierror.NewAPIError(
			http.StatusUnprocessableEntity,
			errors.New("only one asset is allowed per product variant"),
		), true
	}

	switch db.ErrorCode(err) {
	case db.ForeignKeyViolation:
		return apierror.NewAPIError(
			http.StatusUnprocessableEntity,
			errors.New("invalid category, variant, or attribute reference"),
		), true
	case db.CheckViolation:
		return apierror.NewAPIError(
			http.StatusUnprocessableEntity,
			errors.New("invalid product data"),
		), true
	}

	return apierror.APIError{}, false
}

func collectAssetTokens(req *CreateProductRequest) ([]string, error) {
	seen := make(map[string]struct{})
	tokens := make([]string, 0, len(req.Assets)+len(req.Variants))

	add := func(token string) error {
		if _, dup := seen[token]; dup {
			return apierror.NewAPIError(
				http.StatusBadRequest,
				fmt.Errorf("asset token %s used more than once", token),
			)
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
		return nil
	}

	for _, asset := range req.Assets {
		if err := add(asset.Token); err != nil {
			return nil, err
		}
	}
	for _, variant := range req.Variants {
		if variant.Asset == nil {
			continue
		}
		if err := add(variant.Asset.Token); err != nil {
			return nil, err
		}
	}
	return tokens, nil
}

func buildAssetParams(req ProductAssetRequest, pending PendingUpload) db.ProductAssetParams {
	var duration *int16
	if pending.Type == string(util.ProductAssetVideo) && pending.DurationSeconds > 0 {
		d := int16(math.Round(pending.DurationSeconds))
		duration = &d
	}
	return db.ProductAssetParams{
		AssetKey:        pending.FinalKey,
		Type:            pending.Type,
		MimeType:        pending.ContentType,
		AltText:         req.AltText,
		SortOrder:       req.SortOrder,
		IsPrimary:       req.IsPrimary,
		DurationSeconds: duration,
	}
}

func (h *V1Handler) resolvePendingUpload(
	ctx context.Context,
	token string,
	organizationID uuid.UUID,
) (PendingUpload, error) {
	cacheBytes, err := h.cache.GetPendingUpload(ctx, token)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return PendingUpload{}, apierror.NewAPIError(
				http.StatusBadRequest,
				fmt.Errorf("invalid or expired asset token: %s", token),
			)
		}
		return PendingUpload{}, fmt.Errorf("failed to look up token %s: %w", token, err)
	}

	var pending PendingUpload
	if marshallErr := json.Unmarshal(cacheBytes, &pending); marshallErr != nil {
		return PendingUpload{}, fmt.Errorf("failed to unmarshal pending upload: %w", marshallErr)
	}

	if pending.OrganizationID != organizationID {
		return PendingUpload{}, apierror.NewAPIError(
			http.StatusBadRequest,
			fmt.Errorf("invalid or expired asset token: %s", token),
		)
	}

	return pending, nil
}

func (h *V1Handler) cleanupCommittedUpload(parentCtx context.Context, tempKey, token string) {
	//nolint:mnd // fixed timeout
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parentCtx), 15*time.Second)
	defer cancel()

	if err := h.storage.DeleteObject(ctx, *h.bucket, tempKey); err != nil {
		h.logger.WarnContext(
			ctx, "failed to delete temp object after commit",
			slog.String("key", tempKey),
			slog.Any("err", err),
		)
	}
	if err := h.cache.DeletePendingUpload(ctx, token); err != nil {
		h.logger.WarnContext(
			ctx, "failed to delete pending upload cache entry after commit",
			slog.String("token", token),
			slog.Any("err", err),
		)
	}
}

type UpdateProductStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=active draft archived"`
}

type UpdateProductStatusResponse struct {
	ID        uuid.UUID `json:"id"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// UpdateProductStatus godoc
//
//	@Summary		Update product status
//	@Description	Changes a merchant-owned product status.
//	@Tags			products
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string						true	"Product UUID"
//	@Param			request	body		UpdateProductStatusRequest	true	"Status payload"
//	@Success		200		{object}	UpdateProductStatusResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		403		{object}	apierror.APIError
//	@Failure		404		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/products/{id}/status [patch]
func (h *V1Handler) UpdateProductStatus(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	organization, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	rawID := chi.URLParam(r, "id")
	productID, parseErr := uuid.Parse(rawID)
	if parseErr != nil {
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("invalid product id"))
	}

	var req UpdateProductStatusRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	updated, err := h.store.UpdateProductStatus(ctx, db.UpdateProductStatusParams{
		ID:             productID,
		OrganizationID: organization.ID,
		Status:         req.Status,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return apierror.NewAPIError(http.StatusNotFound, errors.New("product not found"))
		}
		return fmt.Errorf("failed to update product status: %w", err)
	}

	return WriteJSON(w, http.StatusOK, UpdateProductStatusResponse{
		ID:        updated.ID,
		Status:    updated.Status,
		UpdatedAt: updated.UpdatedAt,
	})
}

func (h *V1Handler) ResolveAssetURL(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", nil
	}

	if cachedURL, err := h.cache.GetPresignedURL(ctx, key); err == nil && cachedURL != "" {
		return cachedURL, nil
	}

	val, err, _ := h.presignG.Do(key, func() (any, error) {
		// Double check cache under singleflight to minimize S3 hits
		if cachedURL, err := h.cache.GetPresignedURL(ctx, key); err == nil && cachedURL != "" {
			return cachedURL, nil
		}

		presignedURL, err := h.storage.PresignGetObject(ctx, *h.bucket, key, 1*time.Hour)
		if err != nil {
			return "", fmt.Errorf("failed to generate presigned S3 URL: %w", err)
		}

		// Cache lifetime is shorter than the S3 signature so clients pulled near the
		// cache TTL boundary still have headroom.
		//nolint:mnd // 50 minute cache TTL
		_ = h.cache.CachePresignedURL(ctx, key, presignedURL, 50*time.Minute)

		return presignedURL, nil
	})
	if err != nil {
		return "", err
	}

	//nolint:errcheck // prefer to panic when encounter bug
	return val.(string), nil
}

type AssetResponse struct {
	ID              int64   `json:"id"`
	URL             string  `json:"url"`
	Type            string  `json:"type"`
	MimeType        string  `json:"mimeType"`
	AltText         *string `json:"altText"`
	SortOrder       int16   `json:"sortOrder"`
	IsPrimary       bool    `json:"isPrimary"`
	DurationSeconds *int16  `json:"durationSeconds,omitempty"`
}

type AttributeResponse struct {
	AttributeID         int64  `json:"attributeId"`
	AttributeName       string `json:"attributeName"`
	AttributeSlug       string `json:"attributeSlug"`
	AttributeValueID    int64  `json:"attributeValueId"`
	AttributeValue      string `json:"attributeValue"`
	AttributeValueLabel string `json:"attributeValueLabel"`
}

type VariantResponse struct {
	ID             uuid.UUID           `json:"id"`
	Sku            string              `json:"sku"`
	Name           string              `json:"name"`
	Price          decimal.Decimal     `json:"price"`
	TrackInventory bool                `json:"trackInventory"`
	IsActive       bool                `json:"isActive"`
	Attributes     []AttributeResponse `json:"attributes"`
	Asset          *AssetResponse      `json:"asset,omitempty"`
}

type ProductCategoryResponse struct {
	ID       uuid.UUID  `json:"id"`
	ParentID *uuid.UUID `json:"parentId,omitempty"`
	Name     string     `json:"name"`
	Slug     string     `json:"slug"`
}

type ProductDetailsResponse struct {
	ID             uuid.UUID                 `json:"id"`
	OrganizationID uuid.UUID                 `json:"organizationId"`
	CategoryID     uuid.UUID                 `json:"categoryId"`
	CategoryPath   []ProductCategoryResponse `json:"categoryPath,omitempty"`
	Name           string                    `json:"name"`
	Slug           string                    `json:"slug"`
	Description    json.RawMessage           `json:"description"                swaggertype:"object"`
	Status         string                    `json:"status"`
	Specification  json.RawMessage           `json:"specification"              swaggertype:"object"`
	IsFeatured     bool                      `json:"isFeatured"`
	CreatedAt      time.Time                 `json:"createdAt"`
	UpdatedAt      time.Time                 `json:"updatedAt"`
	Assets         []AssetResponse           `json:"assets"`
	Variants       []VariantResponse         `json:"variants"`
}

// GetActiveProductDetails godoc
//
//	@Summary		Get active product details
//	@Description	Returns buyer-visible details for an active product by UUID or slug within an organization.
//	@Tags			storefront
//	@Produce		json
//	@Param			org_id		path		string	true	"Organization UUID"
//	@Param			slug_or_id	path		string	true	"Product slug or UUID"
//	@Success		200			{object}	ProductDetailsResponse
//	@Failure		400			{object}	apierror.APIError
//	@Failure		404			{object}	apierror.APIError
//	@Failure		500			{object}	apierror.APIError
//	@Router			/products/{org_id}/{slug_or_id} [get]
func (h *V1Handler) GetActiveProductDetails( //nolint:funlen
	w http.ResponseWriter,
	r *http.Request,
) error {
	ctx := r.Context()
	orgID, err := uuid.Parse(chi.URLParam(r, "org_id"))
	if err != nil {
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("invalid org_id"))
	}

	slugOrID := chi.URLParam(r, "slug_or_id")
	if slugOrID == "" {
		return apierror.NewAPIError(
			http.StatusBadRequest,
			errors.New("missing slug_or_id parameter"),
		)
	}

	var p db.GetActiveProductByIDRow
	//nolint:nestif // dynamic product query
	if parsedID, parseErr := uuid.Parse(slugOrID); parseErr == nil {
		row, pErr := h.store.GetActiveProductByID(ctx, parsedID)
		if pErr != nil {
			if errors.Is(pErr, db.ErrNotFound) {
				return apierror.NewAPIError(
					http.StatusNotFound,
					fmt.Errorf("product not found: %s", slugOrID),
				)
			}
			return fmt.Errorf("failed to fetch product: %w", pErr)
		}
		if row.OrganizationID != orgID {
			return apierror.NewAPIError(
				http.StatusNotFound,
				fmt.Errorf("product not found: %s", slugOrID),
			)
		}
		p = row
	} else {
		row, pErr := h.store.GetActiveProductBySlug(ctx, db.GetActiveProductBySlugParams{
			OrganizationID: orgID,
			Slug:           slugOrID,
		})
		if pErr != nil {
			if errors.Is(pErr, db.ErrNotFound) {
				return apierror.NewAPIError(
					http.StatusNotFound,
					fmt.Errorf("product not found: %s", slugOrID),
				)
			}
			return fmt.Errorf("failed to fetch product: %w", pErr)
		}
		p = db.GetActiveProductByIDRow(row)
	}

	productID := p.ID
	categoryID := p.CategoryID
	name := p.Name
	slug := p.Slug
	status := p.Status
	isFeatured := p.IsFeatured
	description := p.Description
	specification := p.Specification
	createdAt := p.CreatedAt
	updatedAt := p.UpdatedAt

	variants, err := h.store.ListProductVariantsByProductID(ctx, productID)
	if err != nil {
		return fmt.Errorf("failed to fetch variants: %w", err)
	}

	assets, err := h.store.ListProductAssetsByProductID(ctx, productID)
	if err != nil {
		return fmt.Errorf("failed to fetch assets: %w", err)
	}

	attrs, err := h.store.ListVariantAttributesByProduct(ctx, productID)
	if err != nil {
		return fmt.Errorf("failed to fetch attributes: %w", err)
	}

	categoryPath, err := h.productCategoryPath(ctx, categoryID)
	if err != nil {
		return fmt.Errorf("failed to fetch category path: %w", err)
	}

	urlMap := h.resolveAssetURLsParallel(ctx, assets)

	attrMap := make(map[uuid.UUID][]AttributeResponse, len(attrs))
	for _, a := range attrs {
		attrMap[a.ProductVariantID] = append(attrMap[a.ProductVariantID], AttributeResponse{
			AttributeID:         a.AttributeID,
			AttributeName:       a.AttributeName,
			AttributeSlug:       a.AttributeSlug,
			AttributeValueID:    a.AttributeValueID,
			AttributeValue:      a.AttributeValue,
			AttributeValueLabel: a.AttributeValueLabel,
		})
	}

	var productAssets []AssetResponse
	variantAssetMap := make(map[uuid.UUID]AssetResponse, len(assets))
	for _, a := range assets {
		resp := AssetResponse{
			ID:              a.ID,
			URL:             urlMap[a.AssetKey],
			Type:            a.Type,
			MimeType:        a.MimeType,
			AltText:         a.AltText,
			SortOrder:       a.SortOrder,
			IsPrimary:       a.IsPrimary,
			DurationSeconds: a.DurationSeconds,
		}
		if a.ProductVariantID == nil {
			productAssets = append(productAssets, resp)
		} else {
			variantAssetMap[*a.ProductVariantID] = resp
		}
	}

	var variantResponses []VariantResponse
	for _, v := range variants {
		var variantAsset *AssetResponse
		if va, exists := variantAssetMap[v.ID]; exists {
			variantAsset = &va
		}
		variantResponses = append(variantResponses, VariantResponse{
			ID:             v.ID,
			Sku:            v.Sku,
			Name:           v.Name,
			Price:          v.Price,
			TrackInventory: v.TrackInventory,
			IsActive:       v.IsActive,
			Attributes:     attrMap[v.ID],
			Asset:          variantAsset,
		})
	}

	res := ProductDetailsResponse{
		ID:             productID,
		OrganizationID: orgID,
		CategoryID:     categoryID,
		CategoryPath:   categoryPath,
		Name:           name,
		Slug:           slug,
		Description:    json.RawMessage(description),
		Status:         status,
		Specification:  json.RawMessage(specification),
		IsFeatured:     isFeatured,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		Assets:         productAssets,
		Variants:       variantResponses,
	}

	return WriteJSON(w, http.StatusOK, res)
}

func (h *V1Handler) productCategoryPath(
	ctx context.Context,
	categoryID uuid.UUID,
) ([]ProductCategoryResponse, error) {
	categories, err := h.store.ListCategoryPath(ctx, categoryID)
	if err != nil {
		return nil, err
	}

	resp := make([]ProductCategoryResponse, len(categories))
	for i, category := range categories {
		resp[i] = ProductCategoryResponse{
			ID:       category.ID,
			ParentID: category.ParentID,
			Name:     category.Name,
			Slug:     category.Slug,
		}
	}

	return resp, nil
}

type ListProductsResponse struct {
	Data       []ProductDetailsResponse `json:"data"`
	NextCursor *string                  `json:"nextCursor"`
}

// productListRow is a common adaptor for the two distinct sqlc row types returned by
// listing queries. Both queries select the same columns; this type lets the shared
// assembly loop work without duplicating it per query.
type productListRow struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	CategoryID     uuid.UUID
	Name           string
	Slug           string
	Description    []byte
	Status         string
	IsFeatured     bool
	Specification  []byte
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func buildProductResponseList(
	products []productListRow,
	variantsByProduct map[uuid.UUID][]db.ProductVariant,
	attrsByVariant map[uuid.UUID][]AttributeResponse,
	productAssetsByProduct map[uuid.UUID][]AssetResponse,
	variantAssetByVariant map[uuid.UUID]AssetResponse,
) []ProductDetailsResponse {
	resList := make([]ProductDetailsResponse, len(products))
	for i, product := range products {
		productVariants := variantsByProduct[product.ID]
		variantResponses := make([]VariantResponse, len(productVariants))
		for j, v := range productVariants {
			var variantAsset *AssetResponse
			if va, ok := variantAssetByVariant[v.ID]; ok {
				variantAsset = &va
			}
			variantResponses[j] = VariantResponse{
				ID:             v.ID,
				Sku:            v.Sku,
				Name:           v.Name,
				Price:          v.Price,
				TrackInventory: v.TrackInventory,
				IsActive:       v.IsActive,
				Attributes:     attrsByVariant[v.ID],
				Asset:          variantAsset,
			}
		}
		resList[i] = ProductDetailsResponse{
			ID:             product.ID,
			OrganizationID: product.OrganizationID,
			CategoryID:     product.CategoryID,
			Name:           product.Name,
			Slug:           product.Slug,
			Description:    json.RawMessage(product.Description),
			Status:         product.Status,
			Specification:  json.RawMessage(product.Specification),
			IsFeatured:     product.IsFeatured,
			CreatedAt:      product.CreatedAt,
			UpdatedAt:      product.UpdatedAt,
			Assets:         productAssetsByProduct[product.ID],
			Variants:       variantResponses,
		}
	}
	return resList
}

// ListActiveProducts godoc
//
//	@Summary		List active products
//	@Description	Lists buyer-visible active products across merchants using cursor pagination. When q is provided, returns ranked search results using a search cursor. Search cursors snapshot the rank from the previous page, so live product edits between page requests may shift result membership or ordering.
//	@Tags			storefront
//	@Produce		json
//	@Param			limit	query		int		false	"Page size"	minimum(1)	maximum(100)	default(20)
//	@Param			cursor	query		string	false	"Opaque cursor returned from the previous page"
//	@Param			q		query		string	false	"Full-text product search query"	maxlength(128)
//	@Success		200		{object}	ListProductsResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Router			/products [get]
func (h *V1Handler) ListActiveProducts(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	limit := parseLimit(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q != "" {
		return h.searchActiveProducts(w, r, q, limit)
	}

	afterCreatedAt, afterID, cursorErr := decodeCursor(r.URL.Query().Get("cursor"))
	if cursorErr != nil {
		return apierror.NewAPIError(
			http.StatusBadRequest,
			fmt.Errorf("invalid cursor: %w", cursorErr),
		)
	}

	products, err := h.store.ListActiveProductsAfter(ctx, db.ListActiveProductsAfterParams{
		AfterCreatedAt: afterCreatedAt,
		AfterID:        afterID,
		PageLimit:      limit,
	})
	if err != nil {
		return fmt.Errorf("failed to list products: %w", err)
	}

	var nextCursor *string
	if len(products) == int(limit) && len(products) > 0 {
		last := products[len(products)-1]
		encoded := encodeCursor(last.CreatedAt, last.ID)
		nextCursor = &encoded
	}

	rows := make([]productListRow, len(products))
	for i, p := range products {
		rows[i] = productListRow{
			ID: p.ID, OrganizationID: p.OrganizationID, CategoryID: p.CategoryID,
			Name: p.Name, Slug: p.Slug, Description: p.Description,
			Status: p.Status, IsFeatured: p.IsFeatured, Specification: p.Specification,
			CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		}
	}

	return h.writeProductListResponse(ctx, w, rows, nextCursor)
}

func (h *V1Handler) searchActiveProducts(
	w http.ResponseWriter,
	r *http.Request,
	query string,
	limit int32,
) error {
	if len(query) > maxSearchQueryLength {
		return apierror.NewAPIError(
			http.StatusUnprocessableEntity,
			errors.New("search query too long"),
		)
	}

	afterRank, afterCreatedAt, afterID, cursorErr := decodeSearchCursor(r.URL.Query().Get("cursor"))
	if cursorErr != nil {
		return apierror.NewAPIError(
			http.StatusBadRequest,
			fmt.Errorf("invalid search cursor: %w", cursorErr),
		)
	}

	products, err := h.store.SearchProducts(r.Context(), db.SearchProductsParams{
		AfterRank:      afterRank,
		AfterCreatedAt: afterCreatedAt,
		AfterID:        afterID,
		Query:          query,
		PageLimit:      limit,
	})
	if err != nil {
		return fmt.Errorf("failed to search products: %w", err)
	}

	var nextCursor *string
	if len(products) == int(limit) && len(products) > 0 {
		last := products[len(products)-1]
		encoded := encodeSearchCursor(last.Rank, last.CreatedAt, last.ID)
		nextCursor = &encoded
	}

	rows := make([]productListRow, len(products))
	for i, p := range products {
		rows[i] = productListRow{
			ID: p.ID, OrganizationID: p.OrganizationID, CategoryID: p.CategoryID,
			Name: p.Name, Slug: p.Slug, Description: p.Description,
			Status: p.Status, IsFeatured: p.IsFeatured, Specification: p.Specification,
			CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		}
	}

	return h.writeProductListResponse(r.Context(), w, rows, nextCursor)
}

func (h *V1Handler) writeProductListResponse(
	ctx context.Context,
	w http.ResponseWriter,
	products []productListRow,
	nextCursor *string,
) error {
	if len(products) == 0 {
		return WriteJSON(w, http.StatusOK, ListProductsResponse{
			Data:       []ProductDetailsResponse{},
			NextCursor: nextCursor,
		})
	}

	productIDs := make([]uuid.UUID, len(products))
	for i, p := range products {
		productIDs[i] = p.ID
	}

	variants, err := h.store.ListProductVariantsByProductIDs(ctx, productIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch variants: %w", err)
	}
	assets, err := h.store.ListProductAssetsByProductIDs(ctx, productIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch assets: %w", err)
	}
	attrs, err := h.store.ListVariantAttributesByProductIDs(ctx, productIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch attributes: %w", err)
	}

	urlMap := h.resolveAssetURLsParallel(ctx, assets)

	variantsByProduct := make(map[uuid.UUID][]db.ProductVariant, len(products))
	for _, v := range variants {
		variantsByProduct[v.ProductID] = append(variantsByProduct[v.ProductID], v)
	}

	attrsByVariant := make(map[uuid.UUID][]AttributeResponse, len(attrs))
	for _, a := range attrs {
		attrsByVariant[a.ProductVariantID] = append(
			attrsByVariant[a.ProductVariantID],
			AttributeResponse{
				AttributeID:         a.AttributeID,
				AttributeName:       a.AttributeName,
				AttributeSlug:       a.AttributeSlug,
				AttributeValueID:    a.AttributeValueID,
				AttributeValue:      a.AttributeValue,
				AttributeValueLabel: a.AttributeValueLabel,
			},
		)
	}

	productAssetsByProduct := make(map[uuid.UUID][]AssetResponse, len(products))
	variantAssetByVariant := make(map[uuid.UUID]AssetResponse, len(assets))
	for _, a := range assets {
		resp := AssetResponse{
			ID:              a.ID,
			URL:             urlMap[a.AssetKey],
			Type:            a.Type,
			MimeType:        a.MimeType,
			AltText:         a.AltText,
			SortOrder:       a.SortOrder,
			IsPrimary:       a.IsPrimary,
			DurationSeconds: a.DurationSeconds,
		}
		if a.ProductVariantID == nil {
			productAssetsByProduct[a.ProductID] = append(productAssetsByProduct[a.ProductID], resp)
		} else {
			variantAssetByVariant[*a.ProductVariantID] = resp
		}
	}

	return WriteJSON(w, http.StatusOK, ListProductsResponse{
		Data: buildProductResponseList(
			products,
			variantsByProduct,
			attrsByVariant,
			productAssetsByProduct,
			variantAssetByVariant,
		),
		NextCursor: nextCursor,
	})
}

// resolveAssetURLsParallel dedupes asset keys and resolves them concurrently with
// bounded fan-out. Failures are swallowed (best-effort); the affected entry is
// simply omitted from the returned map, which leaves AssetResponse.URL as "".
func (h *V1Handler) resolveAssetURLsParallel(
	ctx context.Context,
	assets []db.ProductAsset,
) map[string]string {
	uniqueKeys := make(map[string]struct{}, len(assets))
	for _, a := range assets {
		uniqueKeys[a.AssetKey] = struct{}{}
	}

	urlMap := make(map[string]string, len(uniqueKeys))
	if len(uniqueKeys) == 0 {
		return urlMap
	}

	var urlMu sync.Mutex
	const presignConcurrency = 8
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(presignConcurrency)

	for key := range uniqueKeys {
		g.Go(func() error {
			url, presignErr := h.ResolveAssetURL(gCtx, key)
			if presignErr != nil {
				h.logger.WarnContext(
					gCtx, "failed to resolve asset URL",
					slog.String("assetKey", key),
					slog.Any("err", presignErr),
				)
				return nil
			}
			urlMu.Lock()
			urlMap[key] = url
			urlMu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	return urlMap
}

const (
	defaultListLimit            = 20
	maxListLimit                = 100
	maxSearchQueryLength        = 128
	productAssetCopyConcurrency = 8
)

func parseLimit(r *http.Request) int32 {
	limit := int32(defaultListLimit)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			if parsed > maxListLimit {
				parsed = maxListLimit
			}
			//nolint:gosec // parsed is capped to maxListLimit (100) before this conversion
			limit = int32(
				parsed,
			)
		}
	}
	return limit
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%d:%s", t.UnixNano(), id.String())
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func encodeSearchCursor(rank float64, t time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf(
		"%s:%d:%s",
		strconv.FormatFloat(rank, 'g', -1, 64),
		t.UnixNano(),
		id.String(),
	)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (time.Time, uuid.UUID, error) {
	if cursor == "" {
		// Sentinel: returns from the newest row. Far-future timestamp ensures the
		// (created_at, id) < (...) predicate matches every active row.
		return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), uuid.Max, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, errors.New("malformed cursor encoding")
	}

	//nolint:mnd // exactly 2 parts
	parts := strings.SplitN(string(raw), ":", 2)
	//nolint:mnd // exactly 2 parts
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, errors.New("malformed cursor payload")
	}

	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, uuid.Nil, errors.New("malformed cursor timestamp")
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, errors.New("malformed cursor id")
	}

	return time.Unix(0, nanos).UTC(), id, nil
}

func decodeSearchCursor(cursor string) (float64, time.Time, uuid.UUID, error) {
	if cursor == "" {
		return math.Inf(1), time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), uuid.Max, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, time.Time{}, uuid.Nil, errors.New("malformed cursor encoding")
	}

	//nolint:mnd // exactly 3 parts
	parts := strings.SplitN(string(raw), ":", 3)
	//nolint:mnd // exactly 3 parts
	if len(parts) != 3 {
		return 0, time.Time{}, uuid.Nil, errors.New("malformed cursor payload")
	}

	rank, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, time.Time{}, uuid.Nil, errors.New("malformed cursor rank")
	}

	nanos, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, time.Time{}, uuid.Nil, errors.New("malformed cursor timestamp")
	}

	id, err := uuid.Parse(parts[2])
	if err != nil {
		return 0, time.Time{}, uuid.Nil, errors.New("malformed cursor id")
	}

	return rank, time.Unix(0, nanos).UTC(), id, nil
}

const (
	idempotencyLockTTL  = 2 * time.Minute
	idempotencyCacheTTL = 24 * time.Hour
)

type idempotencyCacheEntry struct {
	RequestHash string                    `json:"requestHash"`
	Result      db.CreateProductTxResults `json:"result"`
}

func (h *V1Handler) lookupIdempotentResultFromCache(
	ctx context.Context,
	organizationID uuid.UUID,
	idempotencyKey, requestHash string,
) (db.CreateProductTxResults, bool, error) {
	raw, err := h.cache.GetIdempotentProductResult(ctx, organizationID, idempotencyKey)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			h.logger.WarnContext(
				ctx, "idempotency cache lookup failed",
				slog.String("idempotencyKey", idempotencyKey),
				slog.Any("err", err),
			)
		}
		return db.CreateProductTxResults{}, false, nil
	}
	if len(raw) == 0 {
		return db.CreateProductTxResults{}, false, nil
	}

	var entry idempotencyCacheEntry
	if unmarshalErr := json.Unmarshal(raw, &entry); unmarshalErr != nil {
		//nolint:nilerr // Corrupt cache entry — fall through to DB path rather than 422.
		return db.CreateProductTxResults{}, false, nil
	}
	if entry.RequestHash != requestHash {
		return db.CreateProductTxResults{}, false, apierror.NewAPIError(
			http.StatusUnprocessableEntity,
			errors.New("idempotency key reused with a different request payload"),
		)
	}
	return entry.Result, true, nil
}

// lookupIdempotentResultFromDB returns the previously-persisted product for the
// given (org, idempotency_key). Payload-mismatch detection lives only in the
// cache path; once the cache TTL elapses, we trust controlled clients not to
// reuse a key with a different payload.
func (h *V1Handler) lookupIdempotentResultFromDB(
	ctx context.Context,
	organizationID uuid.UUID,
	idempotencyKey string,
) (db.CreateProductTxResults, bool, error) {
	keyPtr := idempotencyKey
	product, err := h.store.GetProductByIdempotencyKey(ctx, db.GetProductByIdempotencyKeyParams{
		OrganizationID: organizationID,
		IdempotencyKey: &keyPtr,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.CreateProductTxResults{}, false, nil
		}
		return db.CreateProductTxResults{}, false, fmt.Errorf("idempotency DB lookup: %w", err)
	}

	productIDs := []uuid.UUID{product.ID}
	variants, vErr := h.store.ListProductVariantsByProductIDs(ctx, productIDs)
	if vErr != nil {
		return db.CreateProductTxResults{}, false, fmt.Errorf("reconstruct variants: %w", vErr)
	}
	assets, aErr := h.store.ListProductAssetsByProductIDs(ctx, productIDs)
	if aErr != nil {
		return db.CreateProductTxResults{}, false, fmt.Errorf("reconstruct assets: %w", aErr)
	}

	return db.CreateProductTxResults{
		Product:         product,
		ProductVariants: variants,
		ProductAssets:   assets,
	}, true, nil
}

func (h *V1Handler) cacheIdempotentResult(
	ctx context.Context,
	organizationID uuid.UUID,
	idempotencyKey, requestHash string,
	result db.CreateProductTxResults,
) {
	entry := idempotencyCacheEntry{RequestHash: requestHash, Result: result}
	raw, err := json.Marshal(entry)
	if err != nil {
		h.logger.WarnContext(
			ctx, "failed to marshal idempotency cache entry",
			slog.Any("err", err),
		)
		return
	}
	if setErr := h.cache.CacheIdempotentProductResult(
		ctx, organizationID, idempotencyKey, raw, idempotencyCacheTTL,
	); setErr != nil {
		h.logger.WarnContext(
			ctx, "failed to write idempotency cache entry",
			slog.String("idempotencyKey", idempotencyKey),
			slog.Any("err", setErr),
		)
	}
}

// ListMerchantProducts godoc
//
//	@Summary		List merchant products
//	@Description	Lists products belonging to the authenticated merchant organization.
//	@Tags			products
//	@Produce		json
//	@Param			status	query		string	false	"Comma-separated status filter"	Enums(active,draft,archived,suspended)
//	@Param			limit	query		int		false	"Page size"						minimum(1)	maximum(100)	default(20)
//	@Param			cursor	query		string	false	"Opaque cursor returned from the previous page"
//	@Success		200		{object}	ListProductsResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		403		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/merchant/products [get]
func (h *V1Handler) ListMerchantProducts( //nolint:funlen
	w http.ResponseWriter,
	r *http.Request,
) error {
	ctx := r.Context()
	org, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	limit := parseLimit(r)

	afterCreatedAt, afterID, cursorErr := decodeCursor(r.URL.Query().Get("cursor"))
	if cursorErr != nil {
		return apierror.NewAPIError(
			http.StatusBadRequest,
			fmt.Errorf("invalid cursor: %w", cursorErr),
		)
	}

	allowedStatuses := map[string]struct{}{
		"active":    {},
		"draft":     {},
		"archived":  {},
		"suspended": {},
	}
	statuses := []string{"active", "draft", "archived", "suspended"}
	if rawStatus := r.URL.Query().Get("status"); rawStatus != "" {
		requested := strings.Split(rawStatus, ",")
		for _, s := range requested {
			if _, ok := allowedStatuses[s]; !ok {
				return apierror.NewAPIError(
					http.StatusBadRequest,
					fmt.Errorf(
						"invalid status %q: must be one of active, draft, archived, suspended",
						s,
					),
				)
			}
		}
		statuses = requested
	}

	products, err := h.store.ListProductsByOrganizationWithStatus(
		ctx,
		db.ListProductsByOrganizationWithStatusParams{
			OrganizationID: org.ID,
			Statuses:       statuses,
			AfterCreatedAt: afterCreatedAt,
			AfterID:        afterID,
			PageLimit:      limit,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to list merchant products: %w", err)
	}

	var nextCursor *string
	if len(products) == int(limit) && len(products) > 0 {
		last := products[len(products)-1]
		encoded := encodeCursor(last.CreatedAt, last.ID)
		nextCursor = &encoded
	}

	if len(products) == 0 {
		return WriteJSON(w, http.StatusOK, ListProductsResponse{
			Data:       []ProductDetailsResponse{},
			NextCursor: nextCursor,
		})
	}

	productIDs := make([]uuid.UUID, len(products))
	for i, p := range products {
		productIDs[i] = p.ID
	}

	variants, err := h.store.ListProductVariantsByProductIDs(ctx, productIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch variants: %w", err)
	}
	assets, err := h.store.ListProductAssetsByProductIDs(ctx, productIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch assets: %w", err)
	}
	attrs, err := h.store.ListVariantAttributesByProductIDs(ctx, productIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch attributes: %w", err)
	}

	urlMap := h.resolveAssetURLsParallel(ctx, assets)

	variantsByProduct := make(map[uuid.UUID][]db.ProductVariant, len(products))
	for _, v := range variants {
		variantsByProduct[v.ProductID] = append(variantsByProduct[v.ProductID], v)
	}

	attrsByVariant := make(map[uuid.UUID][]AttributeResponse, len(attrs))
	for _, a := range attrs {
		attrsByVariant[a.ProductVariantID] = append(
			attrsByVariant[a.ProductVariantID],
			AttributeResponse{
				AttributeID:         a.AttributeID,
				AttributeName:       a.AttributeName,
				AttributeSlug:       a.AttributeSlug,
				AttributeValueID:    a.AttributeValueID,
				AttributeValue:      a.AttributeValue,
				AttributeValueLabel: a.AttributeValueLabel,
			},
		)
	}

	productAssetsByProduct := make(map[uuid.UUID][]AssetResponse, len(products))
	variantAssetByVariant := make(map[uuid.UUID]AssetResponse, len(assets))
	for _, a := range assets {
		resp := AssetResponse{
			ID:              a.ID,
			URL:             urlMap[a.AssetKey],
			Type:            a.Type,
			MimeType:        a.MimeType,
			AltText:         a.AltText,
			SortOrder:       a.SortOrder,
			IsPrimary:       a.IsPrimary,
			DurationSeconds: a.DurationSeconds,
		}
		if a.ProductVariantID == nil {
			productAssetsByProduct[a.ProductID] = append(productAssetsByProduct[a.ProductID], resp)
		} else {
			variantAssetByVariant[*a.ProductVariantID] = resp
		}
	}

	rows := make([]productListRow, len(products))
	for i, p := range products {
		rows[i] = productListRow{
			ID: p.ID, OrganizationID: p.OrganizationID, CategoryID: p.CategoryID,
			Name: p.Name, Slug: p.Slug, Description: p.Description,
			Status: p.Status, IsFeatured: p.IsFeatured, Specification: p.Specification,
			CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		}
	}

	return WriteJSON(w, http.StatusOK, ListProductsResponse{
		Data: buildProductResponseList(
			rows,
			variantsByProduct,
			attrsByVariant,
			productAssetsByProduct,
			variantAssetByVariant,
		),
		NextCursor: nextCursor,
	})
}

// GetMerchantProductDetails godoc
//
//	@Summary		Get merchant product details
//	@Description	Returns full details for a merchant-owned product regardless of status.
//	@Tags			products
//	@Produce		json
//	@Param			id	path		string	true	"Product UUID"
//	@Success		200	{object}	ProductDetailsResponse
//	@Failure		400	{object}	apierror.APIError
//	@Failure		401	{object}	apierror.APIError
//	@Failure		403	{object}	apierror.APIError
//	@Failure		404	{object}	apierror.APIError
//	@Failure		500	{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/merchant/products/{id} [get]
func (h *V1Handler) GetMerchantProductDetails( //nolint:funlen
	w http.ResponseWriter,
	r *http.Request,
) error {
	ctx := r.Context()
	org, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	rawID := chi.URLParam(r, "id")
	productID, parseErr := uuid.Parse(rawID)
	if parseErr != nil {
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("invalid product id"))
	}

	row, fetchErr := h.store.GetProductByID(ctx, productID)
	if fetchErr != nil {
		if errors.Is(fetchErr, db.ErrNotFound) {
			return apierror.NewAPIError(http.StatusNotFound, errors.New("product not found"))
		}
		return fmt.Errorf("failed to fetch product: %w", fetchErr)
	}
	if row.OrganizationID != org.ID {
		return apierror.NewAPIError(http.StatusNotFound, errors.New("product not found"))
	}

	organizationID := row.OrganizationID
	categoryID := row.CategoryID
	name := row.Name
	slug := row.Slug
	status := row.Status
	isFeatured := row.IsFeatured
	description := row.Description
	specification := row.Specification
	createdAt := row.CreatedAt
	updatedAt := row.UpdatedAt

	variants, err := h.store.ListProductVariantsByProductID(ctx, productID)
	if err != nil {
		return fmt.Errorf("failed to fetch variants: %w", err)
	}

	assets, err := h.store.ListProductAssetsByProductID(ctx, productID)
	if err != nil {
		return fmt.Errorf("failed to fetch assets: %w", err)
	}

	attrs, err := h.store.ListVariantAttributesByProduct(ctx, productID)
	if err != nil {
		return fmt.Errorf("failed to fetch attributes: %w", err)
	}

	categoryPath, err := h.productCategoryPath(ctx, categoryID)
	if err != nil {
		return fmt.Errorf("failed to fetch category path: %w", err)
	}

	urlMap := h.resolveAssetURLsParallel(ctx, assets)

	attrMap := make(map[uuid.UUID][]AttributeResponse, len(attrs))
	for _, a := range attrs {
		attrMap[a.ProductVariantID] = append(attrMap[a.ProductVariantID], AttributeResponse{
			AttributeID:         a.AttributeID,
			AttributeName:       a.AttributeName,
			AttributeSlug:       a.AttributeSlug,
			AttributeValueID:    a.AttributeValueID,
			AttributeValue:      a.AttributeValue,
			AttributeValueLabel: a.AttributeValueLabel,
		})
	}

	var productAssets []AssetResponse
	variantAssetMap := make(map[uuid.UUID]AssetResponse, len(assets))
	for _, a := range assets {
		resp := AssetResponse{
			ID:              a.ID,
			URL:             urlMap[a.AssetKey],
			Type:            a.Type,
			MimeType:        a.MimeType,
			AltText:         a.AltText,
			SortOrder:       a.SortOrder,
			IsPrimary:       a.IsPrimary,
			DurationSeconds: a.DurationSeconds,
		}
		if a.ProductVariantID == nil {
			productAssets = append(productAssets, resp)
		} else {
			variantAssetMap[*a.ProductVariantID] = resp
		}
	}

	var variantResponses []VariantResponse
	for _, v := range variants {
		var variantAsset *AssetResponse
		if va, exists := variantAssetMap[v.ID]; exists {
			variantAsset = &va
		}
		variantResponses = append(variantResponses, VariantResponse{
			ID:             v.ID,
			Sku:            v.Sku,
			Name:           v.Name,
			Price:          v.Price,
			TrackInventory: v.TrackInventory,
			IsActive:       v.IsActive,
			Attributes:     attrMap[v.ID],
			Asset:          variantAsset,
		})
	}

	res := ProductDetailsResponse{
		ID:             productID,
		OrganizationID: organizationID,
		CategoryID:     categoryID,
		CategoryPath:   categoryPath,
		Name:           name,
		Slug:           slug,
		Description:    json.RawMessage(description),
		Status:         status,
		Specification:  json.RawMessage(specification),
		IsFeatured:     isFeatured,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		Assets:         productAssets,
		Variants:       variantResponses,
	}

	return WriteJSON(w, http.StatusOK, res)
}

func (h *V1Handler) resolveAssetOrToken(
	ctx context.Context,
	token string,
	organizationID uuid.UUID,
	existingAssetsByKey map[string]db.ProductAsset,
) (PendingUpload, error) {
	// Final asset keys are only accepted when they already belong to this product.
	// New assets must flow through pre-upload so MIME validation, media processing,
	// and org ownership checks are applied before product update.
	if strings.HasPrefix(token, "assets/"+organizationID.String()+"/") {
		asset, ok := existingAssetsByKey[token]
		if !ok {
			return PendingUpload{}, apierror.NewAPIError(
				http.StatusBadRequest,
				fmt.Errorf("invalid or expired asset token: %s", token),
			)
		}

		var durationSeconds float64
		if asset.DurationSeconds != nil {
			durationSeconds = float64(*asset.DurationSeconds)
		}

		return PendingUpload{
			Token:           token,
			TempKey:         "",
			FinalKey:        token,
			Type:            asset.Type,
			ContentType:     asset.MimeType,
			OriginalName:    "",
			OrganizationID:  organizationID,
			DurationSeconds: durationSeconds,
		}, nil
	}
	// Otherwise resolve as temporary pre-upload token.
	return h.resolvePendingUpload(ctx, token, organizationID)
}

// UpdateProduct godoc
//
//	@Summary		Update product
//	@Description	Replaces a merchant-owned product's details, variants, and asset associations.
//	@Tags			products
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Product UUID"
//	@Param			request	body		CreateProductRequest	true	"Replacement product payload"
//	@Success		200		{object}	db.CreateProductTxResults
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		403		{object}	apierror.APIError
//	@Failure		404		{object}	apierror.APIError
//	@Failure		413		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/products/{id} [put]
//
//nolint:gocognit,funlen
func (h *V1Handler) UpdateProduct(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var committed bool
	organization, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	rawID := chi.URLParam(r, "id")
	productID, parseErr := uuid.Parse(rawID)
	if parseErr != nil {
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("invalid product id"))
	}

	// Verify existence and ownership before any asset fetching or S3 work.
	currentProduct, fetchErr := h.store.GetProductByID(ctx, productID)
	if fetchErr != nil {
		if errors.Is(fetchErr, db.ErrNotFound) {
			return apierror.NewAPIError(http.StatusNotFound, errors.New("product not found"))
		}
		return fmt.Errorf("failed to load current product: %w", fetchErr)
	}
	if currentProduct.OrganizationID != organization.ID {
		return apierror.NewAPIError(http.StatusNotFound, errors.New("product not found"))
	}

	// Read and decode body (Max 1MB)
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		return apierror.NewAPIError(
			http.StatusRequestEntityTooLarge,
			errors.New("request body too large"),
		)
	}

	var req CreateProductRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return apierror.ErrInvalidJSON()
	}
	if err := h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	// Retrieve existing assets so we can validate final-key reuse and identify
	// orphaned S3 objects to delete after the transaction succeeds.
	existingAssets, err := h.store.ListProductAssetsByProductID(ctx, productID)
	if err != nil {
		return fmt.Errorf("failed to fetch product assets: %w", err)
	}
	existingAssetsByKey := make(map[string]db.ProductAsset, len(existingAssets))
	for _, asset := range existingAssets {
		existingAssetsByKey[asset.AssetKey] = asset
	}

	tokens, err := collectAssetTokens(&req)
	if err != nil {
		return err
	}

	// Resolve each token/key.
	resolvedAssets := make(map[string]PendingUpload, len(tokens))
	for _, token := range tokens {
		pending, resolveErr := h.resolveAssetOrToken(
			ctx,
			token,
			organization.ID,
			existingAssetsByKey,
		)
		if resolveErr != nil {
			return resolveErr
		}
		resolvedAssets[token] = pending
	}

	// Copy temp objects to final locations in parallel; track for rollback.
	var (
		copiedMu   sync.Mutex
		copiedKeys = make([]string, 0, len(resolvedAssets))
	)

	defer func() {
		if committed {
			return
		}
		copiedMu.Lock()
		keys := append([]string(nil), copiedKeys...)
		copiedMu.Unlock()
		if len(keys) == 0 {
			return
		}

		//nolint:mnd // timeout duration
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		for _, destKey := range keys {
			if delErr := h.storage.DeleteObject(cleanupCtx, *h.bucket, destKey); delErr != nil {
				h.logger.WarnContext(
					cleanupCtx, "failed to roll back final S3 object during update rollback",
					slog.String("key", destKey),
					slog.Any("err", delErr),
				)
			}
		}
	}()

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(productAssetCopyConcurrency)
	for _, pending := range resolvedAssets {
		if pending.TempKey == "" {
			continue // Already in final location
		}
		g.Go(func() error {
			if copyErr := h.storage.CopyObject(
				gCtx,
				*h.bucket,
				pending.TempKey,
				pending.FinalKey,
			); copyErr != nil {
				return fmt.Errorf(
					"failed to copy %s to final location: %w",
					pending.TempKey,
					copyErr,
				)
			}
			copiedMu.Lock()
			copiedKeys = append(copiedKeys, pending.FinalKey)
			copiedMu.Unlock()
			return nil
		})
	}
	if waitErr := g.Wait(); waitErr != nil {
		return waitErr
	}

	// Build transaction params.
	txAssets := make([]db.ProductAssetParams, len(req.Assets))
	for i, assetReq := range req.Assets {
		txAssets[i] = buildAssetParams(assetReq, resolvedAssets[assetReq.Token])
	}

	txVariants := make([]db.ProductVariantParams, len(req.Variants))
	for i, variantReq := range req.Variants {
		var txAsset *db.ProductAssetParams
		if variantReq.Asset != nil {
			params := buildAssetParams(*variantReq.Asset, resolvedAssets[variantReq.Asset.Token])
			txAsset = &params
		}
		txVariants[i] = db.ProductVariantParams{
			Sku:               variantReq.Sku,
			Name:              variantReq.Name,
			Price:             variantReq.Price,
			AttributeValueIDs: variantReq.AttributeValueIDs,
			Asset:             txAsset,
		}
	}

	txParams := db.UpdateProductTxParams{
		ProductID:      productID,
		OrganizationID: organization.ID,
		CategoryID:     req.CategoryID,
		Name:           req.Name,
		Slug:           req.Slug,
		Description:    req.Description,
		Specification:  req.Specification,
		Status:         currentProduct.Status,
		IsFeatured:     currentProduct.IsFeatured,
		Variants:       txVariants,
		Assets:         txAssets,
	}

	// Run update transaction.
	result, err := h.store.UpdateProductTx(ctx, txParams)
	if err != nil {
		if apiErr, ok := productTxAPIError(err); ok {
			return apiErr
		}
		return fmt.Errorf("failed to update product details: %w", err)
	}
	committed = true

	// Post-success S3/Redis cleanup.
	for _, pending := range resolvedAssets {
		if pending.TempKey != "" {
			go h.cleanupCommittedUpload(ctx, pending.TempKey, pending.Token)
		}
	}

	unusedAssetKeys := unusedProductAssetKeys(existingAssets, result.ProductAssets)
	if len(unusedAssetKeys) > 0 {
		go h.cleanupUnusedAssetKeys(ctx, unusedAssetKeys)
	}

	return WriteJSON(w, http.StatusOK, result)
}

func unusedProductAssetKeys(
	existingAssets []db.ProductAsset,
	currentAssets []db.ProductAsset,
) []string {
	usedAssetKeys := make(map[string]struct{}, len(currentAssets))
	for _, asset := range currentAssets {
		usedAssetKeys[asset.AssetKey] = struct{}{}
	}

	unusedAssetKeys := make([]string, 0, len(existingAssets))
	for _, asset := range existingAssets {
		if _, used := usedAssetKeys[asset.AssetKey]; !used {
			unusedAssetKeys = append(unusedAssetKeys, asset.AssetKey)
		}
	}
	return unusedAssetKeys
}

func (h *V1Handler) cleanupUnusedAssetKeys(parentCtx context.Context, keys []string) {
	//nolint:mnd // timeout duration
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(parentCtx), 10*time.Second)
	defer cancel()

	for _, key := range keys {
		if err := h.storage.DeleteObject(cleanupCtx, *h.bucket, key); err != nil {
			h.logger.WarnContext(
				cleanupCtx,
				"failed to delete unused product asset",
				slog.String("key", key),
				slog.Any("err", err),
			)
		}
	}
}

// DeleteProduct godoc
//
//	@Summary		Archive product
//	@Description	Archives a merchant-owned product.
//	@Tags			products
//	@Param			id	path	string	true	"Product UUID"
//	@Success		204	"No content"
//	@Failure		400	{object}	apierror.APIError
//	@Failure		401	{object}	apierror.APIError
//	@Failure		403	{object}	apierror.APIError
//	@Failure		404	{object}	apierror.APIError
//	@Failure		500	{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/products/{id} [delete]
func (h *V1Handler) DeleteProduct(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	organization, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	rawID := chi.URLParam(r, "id")
	productID, parseErr := uuid.Parse(rawID)
	if parseErr != nil {
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("invalid product id"))
	}

	// Verify existence and ownership
	product, fetchErr := h.store.GetProductByID(ctx, productID)
	if fetchErr != nil {
		if errors.Is(fetchErr, db.ErrNotFound) {
			return apierror.NewAPIError(http.StatusNotFound, errors.New("product not found"))
		}
		return fmt.Errorf("failed to fetch product: %w", fetchErr)
	}

	if product.OrganizationID != organization.ID {
		return apierror.NewAPIError(http.StatusNotFound, errors.New("product not found"))
	}

	// Archive the product instead of hard-deleting it, so carts and future order/history flows can
	// continue to resolve product and variant details. The archive query also deactivates variants
	// to keep product and variant availability consistent.
	err := h.store.DeleteProduct(ctx, db.DeleteProductParams{
		ID:             productID,
		OrganizationID: organization.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to archive product: %w", err)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}
