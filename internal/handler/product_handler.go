package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

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
	organization, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	var req CreateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierror.ErrInvalidJSON()
	}

	if err := h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
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
		committed  bool
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
		Variants:       txVariants,
		Assets:         txAssets,
	}

	// 4. Run atomic creation transaction. Failure triggers deferred S3 rollback.
	result, err := h.store.CreateProductTx(ctx, txParams)
	if err != nil {
		return fmt.Errorf("failed to save product details: %w", err)
	}
	committed = true

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
