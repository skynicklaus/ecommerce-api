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

type ProductAssetReq struct {
	Token     string  `json:"token"             validate:"required"`
	IsPrimary bool    `json:"isPrimary"`
	AltText   *string `json:"altText,omitempty"`
	SortOrder int16   `json:"sortOrder"`
}

type VariantRequest struct {
	Sku               string           `json:"sku"               validate:"required"`
	Name              string           `json:"name"              validate:"required"`
	Price             decimal.Decimal  `json:"price"             validate:"required"`
	AttributeValueIDs []int64          `json:"attributeValueIds" validate:"required"`
	Asset             *ProductAssetReq `json:"asset,omitempty"`
}

type CreateProductRequest struct {
	Name          string            `json:"name"          validate:"required"`
	Slug          string            `json:"slug"          validate:"required"`
	CategoryID    uuid.UUID         `json:"categoryId"    validate:"required"`
	Description   json.RawMessage   `json:"description"   validate:"required"`
	Specification json.RawMessage   `json:"specification" validate:"required"`
	Assets        []ProductAssetReq `json:"assets"`
	Variants      []VariantRequest  `json:"variants"      validate:"required,min=1,dive"`
}

func (h *V1Handler) CreateProduct(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var committed bool
	organization, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	// Read the body fully so we can hash the exact bytes the client sent.
	r.Body = http.MaxBytesReader(w, r.Body, maxCreateProductBodySize)
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
	if idempotencyKey != "" {
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
	if err != nil {
		// Last-resort idempotency: another worker beat us past the Redis lock and persisted first.
		// The partial UNIQUE index on (organization_id, idempotency_key) fires here.
		if idempotencyKey != "" && db.IsUniqueViolation(err) {
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

func buildAssetParams(req ProductAssetReq, pending PendingUpload) db.ProductAssetParams {
	var duration *int16
	if pending.Type == string(util.ProductAssetVideo) {
		d := int16(pending.DurationSeconds)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierror.ErrInvalidJSON()
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

	presignedURL, err := h.storage.PresignGetObject(ctx, *h.bucket, key, 1*time.Hour)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned S3 URL: %w", err)
	}

	// Cache lifetime is shorter than the S3 signature so clients pulled near the
	// cache TTL boundary still have headroom.
	//nolint:mnd // 50 minute cache TTL
	_ = h.cache.CachePresignedURL(ctx, key, presignedURL, 50*time.Minute)

	return presignedURL, nil
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

type ProductDetailsResponse struct {
	ID             uuid.UUID         `json:"id"`
	OrganizationID uuid.UUID         `json:"organizationId"`
	CategoryID     uuid.UUID         `json:"categoryId"`
	Name           string            `json:"name"`
	Slug           string            `json:"slug"`
	Description    json.RawMessage   `json:"description"`
	Status         string            `json:"status"`
	Specification  json.RawMessage   `json:"specification"`
	IsFeatured     bool              `json:"isFeatured"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
	Assets         []AssetResponse   `json:"assets"`
	Variants       []VariantResponse `json:"variants"`
}

func (h *V1Handler) GetProductDetails(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	slugOrID := chi.URLParam(r, "slug_or_id")
	if slugOrID == "" {
		return apierror.NewAPIError(
			http.StatusBadRequest,
			errors.New("missing slug_or_id parameter"),
		)
	}

	var (
		productID      uuid.UUID
		organizationID uuid.UUID
		categoryID     uuid.UUID
		name           string
		slug           string
		status         string
		isFeatured     bool
		description    []byte
		specification  []byte
		createdAt      time.Time
		updatedAt      time.Time
	)

	parsedID, parseErr := uuid.Parse(slugOrID)
	if parseErr == nil {
		p, pErr := h.store.GetActiveProductByID(ctx, parsedID)
		if pErr != nil {
			if errors.Is(pErr, db.ErrNotFound) {
				return apierror.NewAPIError(
					http.StatusNotFound,
					fmt.Errorf("product not found: %s", slugOrID),
				)
			}
			return fmt.Errorf("failed to fetch product: %w", pErr)
		}
		productID = p.ID
		organizationID = p.OrganizationID
		categoryID = p.CategoryID
		name = p.Name
		slug = p.Slug
		status = p.Status
		isFeatured = p.IsFeatured
		description = p.Description
		specification = p.Specification
		createdAt = p.CreatedAt
		updatedAt = p.UpdatedAt
	} else {
		p, pErr := h.store.GetActiveProductBySlug(ctx, slugOrID)
		if pErr != nil {
			if errors.Is(pErr, db.ErrNotFound) {
				return apierror.NewAPIError(
					http.StatusNotFound,
					fmt.Errorf("product not found: %s", slugOrID),
				)
			}
			return fmt.Errorf("failed to fetch product: %w", pErr)
		}
		productID = p.ID
		organizationID = p.OrganizationID
		categoryID = p.CategoryID
		name = p.Name
		slug = p.Slug
		status = p.Status
		isFeatured = p.IsFeatured
		description = p.Description
		specification = p.Specification
		createdAt = p.CreatedAt
		updatedAt = p.UpdatedAt
	}

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

	// Group assets and attributes
	urlMap := make(map[string]string)
	for _, asset := range assets {
		if _, exists := urlMap[asset.AssetKey]; !exists {
			presigned, presignErr := h.ResolveAssetURL(ctx, asset.AssetKey)
			if presignErr == nil {
				urlMap[asset.AssetKey] = presigned
			}
		}
	}

	attrMap := make(map[uuid.UUID][]AttributeResponse)
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
	variantAssetMap := make(map[uuid.UUID]AssetResponse)
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

type ListProductsResponse struct {
	Data       []ProductDetailsResponse `json:"data"`
	NextCursor *string                  `json:"nextCursor"`
}

func (h *V1Handler) ListProducts(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	limit := parseLimit(r)

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
	if int32(len(products)) == limit && len(products) > 0 {
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

	attrsByVariant := make(map[uuid.UUID][]AttributeResponse)
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

	productAssetsByProduct := make(map[uuid.UUID][]AssetResponse)
	variantAssetByVariant := make(map[uuid.UUID]AssetResponse)
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

	return WriteJSON(w, http.StatusOK, ListProductsResponse{
		Data:       resList,
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
	defaultListLimit = 20
	maxListLimit     = 100
)

func parseLimit(r *http.Request) int32 {
	limit := int32(defaultListLimit)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			if parsed > maxListLimit {
				parsed = maxListLimit
			}
			limit = int32(parsed)
		}
	}
	return limit
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%d:%s", t.UnixNano(), id.String())
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

const (
	maxCreateProductBodySize = 1 << 20 // 1 MB
	idempotencyLockTTL       = 2 * time.Minute
	idempotencyCacheTTL      = 24 * time.Hour
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
