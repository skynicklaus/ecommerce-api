//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
	"github.com/skynicklaus/ecommerce-api/internal/middleware"
	"github.com/skynicklaus/ecommerce-api/internal/storage"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestCreateProduct_Integration(t *testing.T) {
	ctx := t.Context()

	// 1. Read configuration from environment
	dbSource := os.Getenv("DB_SOURCE")
	if dbSource == "" {
		t.Skip("DB_SOURCE not set")
	}

	bucketName := os.Getenv("S3_BUCKET")
	if bucketName == "" {
		bucketName = "ecommerce-assets"
	}

	// 2. Initialize real infrastructure clients
	connPool, err := pgxpool.New(ctx, dbSource)
	require.NoError(t, err)
	t.Cleanup(connPool.Close)
	t.Cleanup(func() {
		http.DefaultClient.CloseIdleConnections()
		if tr, ok := http.DefaultTransport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	})

	store := db.NewStore(connPool)
	logger := util.NewLogger()

	redisClient := cache.New(store, logger)
	defer redisClient.Close()

	s3Storage, err := storage.New(ctx)
	require.NoError(t, err)

	// 3. Create the Handler instance
	h := NewV1Handler(store, logger, redisClient, s3Storage).(*V1Handler)

	// Override config params to ensure local development targets are fully matched
	h.bucket = &bucketName

	currentYearMonth := time.Now().Format("2006/01")

	// 4. Seed an Organization
	org, err := store.CreateOrganization(ctx, db.CreateOrganizationParams{
		Name:     "Test E2E Org",
		Type:     "merchant",
		Slug:     "merchant.e2e-org-" + uuid.New().String()[:8],
		Status:   "active",
		ParentID: nil,
		Metadata: nil,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = connPool.Exec(
			context.Background(),
			"DELETE FROM organizations WHERE id = $1",
			org.ID,
		)
	})

	// 5. Seed a Category
	orgID := org.ID
	category, err := store.CreateCategory(ctx, db.CreateCategoryParams{
		OrganizationID: &orgID,
		ParentID:       nil,
		Name:           "Test E2E Category",
		Slug:           "e2e-category-" + uuid.New().String()[:8],
		Description:    nil,
		SortOrder:      1,
	})
	require.NoError(t, err)

	// 6. Seed a Color Attribute and an Attribute Value
	attr, err := store.CreateAttribute(ctx, db.CreateAttributeParams{
		OrganizationID: &orgID,
		Name:           "Color",
		Type:           "text",
	})
	require.NoError(t, err)

	val, err := store.CreateAttributeValue(ctx, db.CreateAttributeValueParams{
		AttributeID: attr.ID,
		Value:       "Red",
	})
	require.NoError(t, err)

	var (
		createdProductID   uuid.UUID
		createdProductSlug string
	)

	t.Run("success_full_e2e_flow", func(t *testing.T) {
		token := uuid.New().String()
		tempKey := fmt.Sprintf("temp/%s.png", token)
		finalKey := fmt.Sprintf("assets/%s/%s/%s.png", org.ID.String(), currentYearMonth, token)

		// A. Put temporary object into S3 (Garage)
		_, err = s3Storage.S3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(tempKey),
			Body:   bytes.NewReader([]byte("fake image data bytes")),
		})
		require.NoError(t, err)

		// B. Cache PendingUpload in Redis
		pending := PendingUpload{
			Token:           token,
			TempKey:         tempKey,
			FinalKey:        finalKey,
			Type:            "image",
			ContentType:     "image/png",
			OriginalName:    "test.png",
			DurationSeconds: 0,
			OrganizationID:  org.ID,
		}
		cacheBytes, err := json.Marshal(pending)
		require.NoError(t, err)

		err = redisClient.CachePendingUpload(ctx, token, cacheBytes)
		require.NoError(t, err)

		// C. Prepare Request Payload
		reqPayload := CreateProductRequest{
			Name:          "E2E Product Title",
			Slug:          "e2e-product-" + uuid.New().String()[:8],
			CategoryID:    category.ID,
			Description:   json.RawMessage(`{"text": "E2E product description detail"}`),
			Specification: json.RawMessage(`{"weight": "1.5kg"}`),
			Assets: []ProductAssetRequest{
				{
					Token:     token,
					IsPrimary: true,
					AltText:   aws.String("Stunning primary display alt"),
					SortOrder: 1,
				},
			},
			Variants: []VariantRequest{
				{
					Sku:               "SKU-E2E-SUCCESS-RED-" + uuid.New().String()[:6],
					Name:              "E2E Red Variant",
					Price:             decimal.NewFromFloat(299.99),
					AttributeValueIDs: []int64{val.ID},
					Asset:             nil,
				},
			},
		}

		bodyBytes, err := json.Marshal(reqPayload)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/v1/products", bytes.NewReader(bodyBytes))
		reqCtx := context.WithValue(req.Context(), middleware.OrganizationContextKey{}, org)
		req = req.WithContext(reqCtx)

		rec := httptest.NewRecorder()

		// D. Invoke Handler
		err = h.CreateProduct(rec, req)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, rec.Code)

		// E. Verify Database entries
		var txResult db.CreateProductTxResults
		err = json.Unmarshal(rec.Body.Bytes(), &txResult)
		require.NoError(t, err)
		require.NotEmpty(t, txResult.Product.ID)
		require.Equal(t, reqPayload.Name, txResult.Product.Name)
		require.Len(t, txResult.ProductVariants, 1)
		require.Equal(t, reqPayload.Variants[0].Sku, txResult.ProductVariants[0].Sku)
		require.Len(t, txResult.ProductAssets, 1)

		createdProductID = txResult.Product.ID
		createdProductSlug = txResult.Product.Slug

		// Publish the product so subsequent storefront reads can find it.
		publishBody, err := json.Marshal(UpdateProductStatusRequest{Status: "active"})
		require.NoError(t, err)

		publishReq := httptest.NewRequest(
			http.MethodPatch,
			"/v1/products/"+createdProductID.String()+"/status",
			bytes.NewReader(publishBody),
		)
		publishReqCtx := context.WithValue(
			publishReq.Context(),
			middleware.OrganizationContextKey{},
			org,
		)
		publishReq = publishReq.WithContext(publishReqCtx)

		rctxPub := chi.NewRouteContext()
		rctxPub.URLParams.Add("id", createdProductID.String())
		publishReq = publishReq.WithContext(
			context.WithValue(publishReq.Context(), chi.RouteCtxKey, rctxPub),
		)

		publishRec := httptest.NewRecorder()
		err = h.UpdateProductStatus(publishRec, publishReq)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, publishRec.Code)

		var publishResp UpdateProductStatusResponse
		err = json.Unmarshal(publishRec.Body.Bytes(), &publishResp)
		require.NoError(t, err)
		require.Equal(t, "active", publishResp.Status)

		// Poll for fire-and-forget background S3/Redis deletions to execute
		require.Eventually(t, func() bool {
			_, tempErr := s3Storage.S3.HeadObject(ctx, &s3.HeadObjectInput{
				Bucket: aws.String(bucketName),
				Key:    aws.String(tempKey),
			})
			_, redisErr := redisClient.GetPendingUpload(ctx, token)
			return tempErr != nil && redisErr != nil
		}, 2*time.Second, 10*time.Millisecond, "S3 temporary asset and Redis pending upload token should be deleted in background")

		// F. Verify S3 permanent object exists
		_, err = s3Storage.S3.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(finalKey),
		})
		require.NoError(t, err, "S3 permanent asset should exist")
	})

	t.Run("failure_triggers_s3_copy_rollback", func(t *testing.T) {
		token := uuid.New().String()
		tempKey := fmt.Sprintf("temp/%s.png", token)
		finalKey := fmt.Sprintf("assets/%s/%s/%s.png", org.ID.String(), currentYearMonth, token)

		// A. Put temporary object into S3
		_, err = s3Storage.S3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(tempKey),
			Body:   bytes.NewReader([]byte("fake image data bytes")),
		})
		require.NoError(t, err)

		// B. Cache PendingUpload in Redis
		pending := PendingUpload{
			Token:           token,
			TempKey:         tempKey,
			FinalKey:        finalKey,
			Type:            "image",
			ContentType:     "image/png",
			OriginalName:    "test.png",
			DurationSeconds: 0,
			OrganizationID:  org.ID,
		}
		cacheBytes, err := json.Marshal(pending)
		require.NoError(t, err)

		err = redisClient.CachePendingUpload(ctx, token, cacheBytes)
		require.NoError(t, err)

		// C. Prepare request with an INVALID Category ID to trigger DB transaction failure
		reqPayload := CreateProductRequest{
			Name:          "E2E Rollback Product",
			Slug:          "e2e-rollback-product-" + uuid.New().String()[:8],
			CategoryID:    uuid.New(), // Random UUID (causes foreign key constraint violation)
			Description:   json.RawMessage(`{"text": "Rollback description"}`),
			Specification: json.RawMessage(`{"weight": "1.0kg"}`),
			Assets: []ProductAssetRequest{
				{
					Token:     token,
					IsPrimary: true,
					AltText:   aws.String("Alt text"),
					SortOrder: 1,
				},
			},
			Variants: []VariantRequest{
				{
					Sku:               "SKU-E2E-ROLLBACK-" + uuid.New().String()[:6],
					Name:              "Rollback Variant",
					Price:             decimal.NewFromFloat(199.99),
					AttributeValueIDs: []int64{val.ID},
					Asset:             nil,
				},
			},
		}

		bodyBytes, err := json.Marshal(reqPayload)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/v1/products", bytes.NewReader(bodyBytes))
		reqCtx := context.WithValue(req.Context(), middleware.OrganizationContextKey{}, org)
		req = req.WithContext(reqCtx)

		rec := httptest.NewRecorder()

		// D. Invoke Handler - should return a client-correctable validation error.
		err = h.CreateProduct(rec, req)
		var apiErr apierror.APIError
		require.True(t, errors.As(err, &apiErr), "expected APIError, got: %v", err)
		require.Equal(t, http.StatusUnprocessableEntity, apiErr.StatusCode)

		// E. Verify S3 permanent object was rolled back (deleted)
		_, err = s3Storage.S3.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(finalKey),
		})
		require.Error(t, err, "S3 permanent asset should have been rolled back and deleted")

		// F. Verify S3 temporary object was preserved (so client can fix request and retry)
		_, err = s3Storage.S3.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(tempKey),
		})
		require.NoError(t, err, "S3 temporary asset should be preserved")

		// Clean up
		_ = s3Storage.DeleteObject(ctx, bucketName, tempKey)
	})

	t.Run("idempotency_caching_and_locking", func(t *testing.T) {
		token := uuid.New().String()
		tempKey := fmt.Sprintf("temp/%s.png", token)
		finalKey := fmt.Sprintf("assets/%s/%s/%s.png", org.ID.String(), currentYearMonth, token)

		// Put S3 temp asset
		_, err = s3Storage.S3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(tempKey),
			Body:   bytes.NewReader([]byte("fake data")),
		})
		require.NoError(t, err)

		// Cache in Redis
		pending := PendingUpload{
			Token:          token,
			TempKey:        tempKey,
			FinalKey:       finalKey,
			Type:           "image",
			ContentType:    "image/png",
			OriginalName:   "test.png",
			OrganizationID: org.ID,
		}
		cacheBytes, err := json.Marshal(pending)
		require.NoError(t, err)
		err = redisClient.CachePendingUpload(ctx, token, cacheBytes)
		require.NoError(t, err)

		// Prepare Request Payload
		reqPayload := CreateProductRequest{
			Name:          "Idempotency Product",
			Slug:          "idem-product-" + uuid.New().String()[:8],
			CategoryID:    category.ID,
			Description:   json.RawMessage(`{"text": "idem"}`),
			Specification: json.RawMessage(`{"idem": "yes"}`),
			Assets: []ProductAssetRequest{
				{
					Token:     token,
					IsPrimary: true,
					AltText:   aws.String("idem alt"),
					SortOrder: 1,
				},
			},
			Variants: []VariantRequest{
				{
					Sku:               "SKU-E2E-IDEM-" + uuid.New().String()[:6],
					Name:              "Idem Variant",
					Price:             decimal.NewFromFloat(9.99),
					AttributeValueIDs: []int64{val.ID},
					Asset:             nil,
				},
			},
		}

		bodyBytes, err := json.Marshal(reqPayload)
		require.NoError(t, err)

		idemKey := "idem-key-" + uuid.New().String()

		// A. First Request - Should succeed with 201 Created
		req1 := httptest.NewRequest(http.MethodPost, "/v1/products", bytes.NewReader(bodyBytes))
		req1.Header.Set("Idempotency-Key", idemKey)
		req1Ctx := context.WithValue(req1.Context(), middleware.OrganizationContextKey{}, org)
		req1 = req1.WithContext(req1Ctx)

		rec1 := httptest.NewRecorder()
		err = h.CreateProduct(rec1, req1)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, rec1.Code)

		var res1 db.CreateProductTxResults
		err = json.Unmarshal(rec1.Body.Bytes(), &res1)
		require.NoError(t, err)

		// B. Second Request (same Idempotency Key) - Should return 200 OK with cached response
		req2 := httptest.NewRequest(http.MethodPost, "/v1/products", bytes.NewReader(bodyBytes))
		req2.Header.Set("Idempotency-Key", idemKey)
		req2Ctx := context.WithValue(req2.Context(), middleware.OrganizationContextKey{}, org)
		req2 = req2.WithContext(req2Ctx)

		rec2 := httptest.NewRecorder()
		err = h.CreateProduct(rec2, req2)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rec2.Code)

		var res2 db.CreateProductTxResults
		err = json.Unmarshal(rec2.Body.Bytes(), &res2)
		require.NoError(t, err)
		require.Equal(t, res1.Product.ID, res2.Product.ID, "Should return identical cached product")

		// C. A distinct idempotency key with a duplicate slug should still return a
		// client-correctable conflict, not run the idempotency fallback and become a 500.
		dupeSlugPayload := reqPayload
		dupeSlugPayload.Assets = nil
		dupeSlugPayload.Variants[0].Sku = "SKU-E2E-IDEM-DUPE-SLUG-" + uuid.New().String()[:6]
		dupeSlugBodyBytes, err := json.Marshal(dupeSlugPayload)
		require.NoError(t, err)

		reqDupeSlug := httptest.NewRequest(
			http.MethodPost,
			"/v1/products",
			bytes.NewReader(dupeSlugBodyBytes),
		)
		reqDupeSlug.Header.Set("Idempotency-Key", "idem-key-dupe-slug-"+uuid.New().String())
		reqDupeSlugCtx := context.WithValue(reqDupeSlug.Context(), middleware.OrganizationContextKey{}, org)
		reqDupeSlug = reqDupeSlug.WithContext(reqDupeSlugCtx)

		recDupeSlug := httptest.NewRecorder()
		err = h.CreateProduct(recDupeSlug, reqDupeSlug)
		var dupeSlugAPIErr apierror.APIError
		require.True(t, errors.As(err, &dupeSlugAPIErr), "expected APIError, got: %v", err)
		require.Equal(t, http.StatusConflict, dupeSlugAPIErr.StatusCode)
		require.Equal(t, "product slug already exists", dupeSlugAPIErr.Message)

		// D. Same Idempotency Key with a different payload should fail with 422.
		differentPayload := reqPayload
		differentPayload.Name = "Idempotency Product Different Payload"
		differentBodyBytes, err := json.Marshal(differentPayload)
		require.NoError(t, err)

		reqMismatch := httptest.NewRequest(
			http.MethodPost,
			"/v1/products",
			bytes.NewReader(differentBodyBytes),
		)
		reqMismatch.Header.Set("Idempotency-Key", idemKey)
		reqMismatchCtx := context.WithValue(reqMismatch.Context(), middleware.OrganizationContextKey{}, org)
		reqMismatch = reqMismatch.WithContext(reqMismatchCtx)

		recMismatch := httptest.NewRecorder()
		err = h.CreateProduct(recMismatch, reqMismatch)
		var mismatchAPIErr apierror.APIError
		require.True(t, errors.As(err, &mismatchAPIErr), "expected APIError, got: %v", err)
		require.Equal(t, http.StatusUnprocessableEntity, mismatchAPIErr.StatusCode)

		// E. Concurrent request lock check - Mock setting locked key in Redis, should fail with 409 Conflict
		idemKeyLock := "idem-key-lock-" + uuid.New().String()
		lockKey := cache.IdempotencyLockKey(org.ID, idemKeyLock)
		_ = redisClient.Set(ctx, lockKey, "processing", 10*time.Second)

		req3 := httptest.NewRequest(http.MethodPost, "/v1/products", bytes.NewReader(bodyBytes))
		req3.Header.Set("Idempotency-Key", idemKeyLock)
		req3Ctx := context.WithValue(req3.Context(), middleware.OrganizationContextKey{}, org)
		req3 = req3.WithContext(req3Ctx)

		rec3 := httptest.NewRecorder()
		err = h.CreateProduct(rec3, req3)
		require.Error(t, err, "Should fail with in-flight lock conflict")

		// Clean up S3
		_ = s3Storage.DeleteObject(ctx, bucketName, finalKey)
	})

	t.Run("resolve_asset_url_with_redis_cache", func(t *testing.T) {
		testKey := "assets/" + org.ID.String() + "/" + currentYearMonth + "/test-asset.png"

		// 1. Initial resolution (cache miss, calls S3 Presigner)
		url1, err := h.ResolveAssetURL(ctx, testKey)
		require.NoError(t, err)
		require.Contains(t, url1, "X-Amz-Signature", "Should generate a valid signed S3 URL")

		// Verify key is cached in Redis
		cacheKey := cache.PresignedURLKey(testKey)
		cachedVal, err := redisClient.Get(ctx, cacheKey).Result()
		require.NoError(t, err)
		require.Equal(t, url1, cachedVal, "Presigned URL should be cached in Redis")

		// 2. Second resolution (cache hit, returns directly from Redis)
		url2, err := h.ResolveAssetURL(ctx, testKey)
		require.NoError(t, err)
		require.Equal(t, url1, url2, "Should retrieve matching presigned URL from cache")
	})

	t.Run("catalog_retrieval_and_details", func(t *testing.T) {
		require.NotEqual(
			t,
			uuid.Nil,
			createdProductID,
			"Should have a valid product ID from previous flow",
		)

		// 1. Get Product Details by ID
		reqID := httptest.NewRequest(http.MethodGet, "/v1/products/"+createdProductID.String(), nil)
		reqIDCtx := context.WithValue(reqID.Context(), middleware.OrganizationContextKey{}, org)
		reqID = reqID.WithContext(reqIDCtx)

		// Set chi URLParam to simulate /products/{org_id}/{slug_or_id} routing
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("org_id", org.ID.String())
		rctx.URLParams.Add("slug_or_id", createdProductID.String())
		reqID = reqID.WithContext(context.WithValue(reqID.Context(), chi.RouteCtxKey, rctx))

		recID := httptest.NewRecorder()
		err = h.GetActiveProductDetails(recID, reqID)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, recID.Code)

		var detailsID ProductDetailsResponse
		err = json.Unmarshal(recID.Body.Bytes(), &detailsID)
		require.NoError(t, err)
		require.Equal(t, createdProductID, detailsID.ID)
		require.Equal(t, "E2E Product Title", detailsID.Name)
		require.NotEmpty(t, detailsID.Assets)
		require.Contains(t, detailsID.Assets[0].URL, "X-Amz-Signature")

		// Assert variants & attributes
		require.NotEmpty(t, detailsID.Variants)
		require.Contains(t, detailsID.Variants[0].Sku, "SKU-E2E-SUCCESS-RED-")
		require.NotEmpty(t, detailsID.Variants[0].Attributes)
		require.Equal(t, "Color", detailsID.Variants[0].Attributes[0].AttributeName)
		require.Equal(t, "Red", detailsID.Variants[0].Attributes[0].AttributeValue)

		// 2. Get Product Details by Slug
		reqSlug := httptest.NewRequest(http.MethodGet, "/v1/products/"+createdProductSlug, nil)
		reqSlugCtx := context.WithValue(reqSlug.Context(), middleware.OrganizationContextKey{}, org)
		reqSlug = reqSlug.WithContext(reqSlugCtx)

		rctxSlug := chi.NewRouteContext()
		rctxSlug.URLParams.Add("org_id", org.ID.String())
		rctxSlug.URLParams.Add("slug_or_id", createdProductSlug)
		reqSlug = reqSlug.WithContext(
			context.WithValue(reqSlug.Context(), chi.RouteCtxKey, rctxSlug),
		)

		recSlug := httptest.NewRecorder()
		err = h.GetActiveProductDetails(recSlug, reqSlug)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, recSlug.Code)

		var detailsSlug ProductDetailsResponse
		err = json.Unmarshal(recSlug.Body.Bytes(), &detailsSlug)
		require.NoError(t, err)
		require.Equal(t, createdProductID, detailsSlug.ID)

		// 3. Get Details for Invalid Slug - Should fail with 404 Not Found
		reqInvalid := httptest.NewRequest(http.MethodGet, "/v1/products/invalid-slug-xyz", nil)
		reqInvalidCtx := context.WithValue(
			reqInvalid.Context(),
			middleware.OrganizationContextKey{},
			org,
		)
		reqInvalid = reqInvalid.WithContext(reqInvalidCtx)

		rctxInv := chi.NewRouteContext()
		rctxInv.URLParams.Add("org_id", org.ID.String())
		rctxInv.URLParams.Add("slug_or_id", "invalid-slug-xyz")
		reqInvalid = reqInvalid.WithContext(
			context.WithValue(reqInvalid.Context(), chi.RouteCtxKey, rctxInv),
		)

		recInvalid := httptest.NewRecorder()
		err = h.GetActiveProductDetails(recInvalid, reqInvalid)
		require.Error(t, err, "Should error on non-existent product")

		// 4. List Products for Organization
		reqList := httptest.NewRequest(http.MethodGet, "/v1/products", nil)
		reqListCtx := context.WithValue(reqList.Context(), middleware.OrganizationContextKey{}, org)
		reqList = reqList.WithContext(reqListCtx)

		recList := httptest.NewRecorder()
		err = h.ListActiveProducts(recList, reqList)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, recList.Code)

		var listResp ListProductsResponse
		err = json.Unmarshal(recList.Body.Bytes(), &listResp)
		require.NoError(t, err)
		require.NotEmpty(t, listResp.Data)

		// Confirm our created product is in the list
		found := false
		for _, p := range listResp.Data {
			if p.ID == createdProductID {
				found = true
				break
			}
		}
		require.True(t, found, "Created product should be present in list response")
	})
}

func TestMerchantCatalogCRUD_Integration(t *testing.T) {
	ctx := t.Context()

	// 1. Read configuration from environment
	dbSource := os.Getenv("DB_SOURCE")
	if dbSource == "" {
		t.Skip("DB_SOURCE not set")
	}
	bucketName := os.Getenv("S3_BUCKET")
	if bucketName == "" {
		bucketName = "ecommerce-assets"
	}

	// 2. Initialize real infrastructure clients
	connPool, err := pgxpool.New(ctx, dbSource)
	require.NoError(t, err)
	t.Cleanup(connPool.Close)
	t.Cleanup(func() {
		http.DefaultClient.CloseIdleConnections()
		if tr, ok := http.DefaultTransport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	})

	store := db.NewStore(connPool)
	logger := util.NewLogger()
	redisClient := cache.New(store, logger)
	defer redisClient.Close()
	s3Storage, err := storage.New(ctx)
	require.NoError(t, err)

	h := NewV1Handler(store, logger, redisClient, s3Storage).(*V1Handler)
	h.bucket = &bucketName

	currentYearMonth := time.Now().Format("2006/01")

	// Seed Organization
	org, err := store.CreateOrganization(ctx, db.CreateOrganizationParams{
		Name:   "Merchant E2E Org",
		Type:   "merchant",
		Slug:   "merchant.crud-org-" + uuid.New().String()[:8],
		Status: "active",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = connPool.Exec(
			context.Background(),
			"DELETE FROM organizations WHERE id = $1",
			org.ID,
		)
	})

	// Seed Category
	category, err := store.CreateCategory(ctx, db.CreateCategoryParams{
		OrganizationID: &org.ID,
		Name:           "E2E Category",
		Slug:           "e2e-cat-" + uuid.New().String()[:8],
		SortOrder:      1,
	})
	require.NoError(t, err)

	// Seed Attribute & Value
	attr, err := store.CreateAttribute(ctx, db.CreateAttributeParams{
		OrganizationID: &org.ID,
		Name:           "Size",
		Type:           "text",
	})
	require.NoError(t, err)
	val, err := store.CreateAttributeValue(ctx, db.CreateAttributeValueParams{
		AttributeID: attr.ID,
		Value:       "Large",
	})
	require.NoError(t, err)

	// 3. Create a product in DRAFT status
	txParams := db.CreateProductTxParams{
		OrganizationID: org.ID,
		CategoryID:     category.ID,
		Name:           "E2E Merchant Catalog CRUD Test",
		Slug:           "e2e-merchant-crud-" + uuid.New().String()[:8],
		Description:    []byte(`{"text":"desc"}`),
		Specification:  []byte(`{"weight":"1kg"}`),
		Variants: []db.ProductVariantParams{
			{
				Sku:               "SKU-CRUD-DRAFT-1-" + uuid.New().String()[:6],
				Name:              "Draft Variant",
				Price:             decimal.NewFromFloat(99.99),
				AttributeValueIDs: []int64{val.ID},
			},
		},
	}
	draftProductResults, err := store.CreateProductTx(ctx, txParams)
	require.NoError(t, err)
	productID := draftProductResults.Product.ID

	t.Run("merchant_list_includes_draft", func(t *testing.T) {
		// Public request (unauthenticated) - should return empty because product is Draft
		reqPub := httptest.NewRequest(http.MethodGet, "/v1/products", nil)
		recPub := httptest.NewRecorder()
		err = h.ListActiveProducts(recPub, reqPub)
		require.NoError(t, err)
		var respPub ListProductsResponse
		err = json.Unmarshal(recPub.Body.Bytes(), &respPub)
		require.NoError(t, err)
		for _, p := range respPub.Data {
			require.NotEqual(t, productID, p.ID)
		}

		// Authenticated merchant request — should include the Draft product.
		reqAuth := httptest.NewRequest(http.MethodGet, "/v1/merchant/products", nil)
		reqAuth = reqAuth.WithContext(
			context.WithValue(reqAuth.Context(), middleware.OrganizationContextKey{}, org),
		)
		recAuth := httptest.NewRecorder()
		err = h.ListMerchantProducts(recAuth, reqAuth)
		require.NoError(t, err)
		var respAuth ListProductsResponse
		err = json.Unmarshal(recAuth.Body.Bytes(), &respAuth)
		require.NoError(t, err)

		found := false
		for _, p := range respAuth.Data {
			if p.ID == productID {
				found = true
				require.Equal(t, string(util.ProductStatusDraft), p.Status)
				break
			}
		}
		require.True(t, found, "Draft product should be listed for authenticated merchant")
	})

	t.Run("merchant_get_details_includes_draft", func(t *testing.T) {
		// Public get - should return 404
		reqPub := httptest.NewRequest(http.MethodGet, "/v1/products/"+productID.String(), nil)
		rctxPub := chi.NewRouteContext()
		rctxPub.URLParams.Add("org_id", org.ID.String())
		rctxPub.URLParams.Add("slug_or_id", productID.String())
		reqPub = reqPub.WithContext(context.WithValue(reqPub.Context(), chi.RouteCtxKey, rctxPub))
		recPub := httptest.NewRecorder()
		err = h.GetActiveProductDetails(recPub, reqPub)
		require.Error(t, err)

		// Merchant get - should return product details
		reqAuth := httptest.NewRequest(
			http.MethodGet,
			"/v1/merchant/products/"+productID.String(),
			nil,
		)
		reqAuth = reqAuth.WithContext(
			context.WithValue(reqAuth.Context(), middleware.OrganizationContextKey{}, org),
		)
		rctxAuth := chi.NewRouteContext()
		rctxAuth.URLParams.Add("id", productID.String())
		reqAuth = reqAuth.WithContext(
			context.WithValue(reqAuth.Context(), chi.RouteCtxKey, rctxAuth),
		)
		recAuth := httptest.NewRecorder()
		err = h.GetMerchantProductDetails(recAuth, reqAuth)
		require.NoError(t, err)

		var details ProductDetailsResponse
		err = json.Unmarshal(recAuth.Body.Bytes(), &details)
		require.NoError(t, err)
		require.Equal(t, productID, details.ID)
		require.Equal(t, string(util.ProductStatusDraft), details.Status)
	})

	t.Run("merchant_update_duplicate_slug_returns_conflict", func(t *testing.T) {
		otherProductResults, err := store.CreateProductTx(ctx, db.CreateProductTxParams{
			OrganizationID: org.ID,
			CategoryID:     category.ID,
			Name:           "Duplicate Slug Target",
			Slug:           "e2e-duplicate-slug-target-" + uuid.New().String()[:8],
			Description:    []byte(`{"text":"dupe target"}`),
			Specification:  []byte(`{"weight":"1kg"}`),
			Variants: []db.ProductVariantParams{
				{
					Sku:               "SKU-DUPE-SLUG-TARGET-" + uuid.New().String()[:6],
					Name:              "Duplicate Slug Target Variant",
					Price:             decimal.NewFromFloat(89.99),
					AttributeValueIDs: []int64{val.ID},
				},
			},
		})
		require.NoError(t, err)

		bodyBytes, err := json.Marshal(CreateProductRequest{
			Name:          "Duplicate Slug Update",
			Slug:          otherProductResults.Product.Slug,
			CategoryID:    category.ID,
			Description:   json.RawMessage(`{"text":"duplicate slug"}`),
			Specification: json.RawMessage(`{"weight":"1kg"}`),
			Variants: []VariantRequest{
				{
					Sku:               draftProductResults.ProductVariants[0].Sku,
					Name:              "Duplicate Slug Variant",
					Price:             decimal.NewFromFloat(149.99),
					AttributeValueIDs: []int64{val.ID},
				},
			},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/v1/products/"+productID.String(),
			bytes.NewReader(bodyBytes),
		)
		reqCtx := context.WithValue(req.Context(), middleware.OrganizationContextKey{}, org)
		req = req.WithContext(reqCtx)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", productID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		err = h.UpdateProduct(rec, req)
		var apiErr apierror.APIError
		require.True(t, errors.As(err, &apiErr), "expected APIError, got: %v", err)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode)
		require.Equal(t, "product slug already exists", apiErr.Message)
	})

	t.Run("merchant_update_product_put", func(t *testing.T) {
		updatePayload := CreateProductRequest{
			Name:          "Updated Product Title",
			Slug:          "e2e-merchant-crud-updated-" + uuid.New().String()[:8],
			CategoryID:    category.ID,
			Description:   json.RawMessage(`{"text": "updated desc"}`),
			Specification: json.RawMessage(`{"weight": "1.2kg"}`),
			Variants: []VariantRequest{
				{
					Sku:               draftProductResults.ProductVariants[0].Sku,
					Name:              "Updated Variant Title",
					Price:             decimal.NewFromFloat(149.99),
					AttributeValueIDs: []int64{val.ID},
				},
				{
					Sku:               "SKU-CRUD-NEW-2-" + uuid.New().String()[:6],
					Name:              "New Added Variant",
					Price:             decimal.NewFromFloat(19.99),
					AttributeValueIDs: []int64{val.ID},
				},
			},
		}

		bodyBytes, err := json.Marshal(updatePayload)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/v1/products/"+productID.String(),
			bytes.NewReader(bodyBytes),
		)
		reqCtx := context.WithValue(req.Context(), middleware.OrganizationContextKey{}, org)
		req = req.WithContext(reqCtx)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", productID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		err = h.UpdateProduct(rec, req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rec.Code)

		var txResult db.CreateProductTxResults
		err = json.Unmarshal(rec.Body.Bytes(), &txResult)
		require.NoError(t, err)

		require.Equal(t, updatePayload.Name, txResult.Product.Name)
		require.Len(t, txResult.ProductVariants, 2)

		var oldSkuVariant db.ProductVariant
		for _, v := range txResult.ProductVariants {
			if v.Sku == draftProductResults.ProductVariants[0].Sku {
				oldSkuVariant = v
				break
			}
		}
		require.NotEmpty(t, oldSkuVariant.ID)
		require.Equal(
			t,
			draftProductResults.ProductVariants[0].ID,
			oldSkuVariant.ID,
			"Variant ID should be preserved",
		)
		require.True(
			t,
			oldSkuVariant.Price.Equal(decimal.NewFromFloat(149.99)),
			"Price should have updated",
		)
	})

	t.Run("cross_org_update_rejected", func(t *testing.T) {
		otherOrg, err := store.CreateOrganization(ctx, db.CreateOrganizationParams{
			Name:   "Other Merchant Org",
			Type:   "merchant",
			Slug:   "merchant.other-org-" + uuid.New().String()[:8],
			Status: "active",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = connPool.Exec(
				context.Background(),
				"DELETE FROM organizations WHERE id = $1",
				otherOrg.ID,
			)
		})

		bodyBytes, err := json.Marshal(CreateProductRequest{
			Name:          "Cross-Org Hijack Attempt",
			Slug:          "hijack-" + uuid.New().String()[:8],
			CategoryID:    category.ID,
			Description:   json.RawMessage(`{"text": "hijack"}`),
			Specification: json.RawMessage(`{"x": "y"}`),
			Variants: []VariantRequest{
				{
					Sku:               "SKU-HIJACK-" + uuid.New().String()[:6],
					Name:              "Hijack Variant",
					Price:             decimal.NewFromFloat(1.00),
					AttributeValueIDs: []int64{val.ID},
				},
			},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/v1/products/"+productID.String(),
			bytes.NewReader(bodyBytes),
		)
		// Inject the other org's context — does not own productID.
		reqCtx := context.WithValue(req.Context(), middleware.OrganizationContextKey{}, otherOrg)
		req = req.WithContext(reqCtx)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", productID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		err = h.UpdateProduct(rec, req)
		var apiErr apierror.APIError
		require.True(t, errors.As(err, &apiErr), "expected APIError, got: %v", err)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	})

	t.Run("merchant_update_removes_variant", func(t *testing.T) {
		// Product currently has 2 variants from merchant_update_product_put.
		// Send an update that omits one — the orphaned variant must be deleted from DB.
		currentVariants, err := store.ListProductVariantsByProductID(ctx, productID)
		require.NoError(t, err)
		require.Len(t, currentVariants, 2)

		keepVariant := currentVariants[0]
		dropVariantID := currentVariants[1].ID

		bodyBytes, err := json.Marshal(CreateProductRequest{
			Name:          "Updated Product Title",
			Slug:          "e2e-merchant-crud-trimmed-" + uuid.New().String()[:8],
			CategoryID:    category.ID,
			Description:   json.RawMessage(`{"text": "trimmed"}`),
			Specification: json.RawMessage(`{"weight": "1.0kg"}`),
			Variants: []VariantRequest{
				{
					Sku:               keepVariant.Sku,
					Name:              keepVariant.Name,
					Price:             keepVariant.Price,
					AttributeValueIDs: []int64{val.ID},
				},
			},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/v1/products/"+productID.String(),
			bytes.NewReader(bodyBytes),
		)
		reqCtx := context.WithValue(req.Context(), middleware.OrganizationContextKey{}, org)
		req = req.WithContext(reqCtx)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", productID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		err = h.UpdateProduct(rec, req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rec.Code)

		remaining, err := store.ListProductVariantsByProductID(ctx, productID)
		require.NoError(t, err)
		require.Len(t, remaining, 1, "dropped variant should have been deleted from DB")
		require.Equal(
			t,
			keepVariant.ID,
			remaining[0].ID,
			"surviving variant ID should be unchanged",
		)
		for _, v := range remaining {
			require.NotEqual(t, dropVariantID, v.ID, "dropped variant must not remain in DB")
		}
	})

	t.Run("merchant_update_reuses_existing_video_asset_key", func(t *testing.T) {
		videoProductResults, err := store.CreateProductTx(ctx, db.CreateProductTxParams{
			OrganizationID: org.ID,
			CategoryID:     category.ID,
			Name:           "Video Reuse Product",
			Slug:           "video-reuse-" + uuid.New().String()[:8],
			Description:    []byte(`{"text":"video reuse"}`),
			Specification:  []byte(`{"weight":"1kg"}`),
			Variants: []db.ProductVariantParams{
				{
					Sku:               "SKU-VIDEO-REUSE-" + uuid.New().String()[:6],
					Name:              "Video Reuse Variant",
					Price:             decimal.NewFromFloat(49.99),
					AttributeValueIDs: []int64{val.ID},
				},
			},
		})
		require.NoError(t, err)

		videoKey := fmt.Sprintf(
			"assets/%s/%s/reused-video-%s.mp4",
			org.ID.String(),
			currentYearMonth,
			uuid.New().String()[:8],
		)
		_, err = s3Storage.S3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(videoKey),
			Body:   bytes.NewReader([]byte("existing video bytes")),
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = s3Storage.DeleteObject(context.Background(), bucketName, videoKey) })

		duration := int16(12)
		_, err = store.CreateProductAsset(ctx, db.CreateProductAssetParams{
			ProductID:        videoProductResults.Product.ID,
			ProductVariantID: nil,
			AssetKey:         videoKey,
			Type:             string(util.ProductAssetVideo),
			MimeType:         "video/mp4",
			AltText:          nil,
			SortOrder:        1,
			IsPrimary:        false,
			DurationSeconds:  &duration,
		})
		require.NoError(t, err)

		bodyBytes, err := json.Marshal(CreateProductRequest{
			Name:          "Video Reuse Product Updated",
			Slug:          "video-reuse-updated-" + uuid.New().String()[:8],
			CategoryID:    category.ID,
			Description:   json.RawMessage(`{"text":"video reuse updated"}`),
			Specification: json.RawMessage(`{"weight":"1kg"}`),
			Assets: []ProductAssetRequest{
				{
					Token:     videoKey,
					SortOrder: 1,
				},
			},
			Variants: []VariantRequest{
				{
					Sku:               videoProductResults.ProductVariants[0].Sku,
					Name:              "Video Reuse Variant Updated",
					Price:             decimal.NewFromFloat(59.99),
					AttributeValueIDs: []int64{val.ID},
				},
			},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/v1/products/"+videoProductResults.Product.ID.String(),
			bytes.NewReader(bodyBytes),
		)
		req = req.WithContext(
			context.WithValue(req.Context(), middleware.OrganizationContextKey{}, org),
		)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", videoProductResults.Product.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		err = h.UpdateProduct(rec, req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rec.Code)

		var txResult db.CreateProductTxResults
		err = json.Unmarshal(rec.Body.Bytes(), &txResult)
		require.NoError(t, err)
		require.Len(t, txResult.ProductAssets, 1)
		require.Equal(t, string(util.ProductAssetVideo), txResult.ProductAssets[0].Type)
		require.Equal(t, "video/mp4", txResult.ProductAssets[0].MimeType)
		require.NotNil(t, txResult.ProductAssets[0].DurationSeconds)
		require.Equal(t, duration, *txResult.ProductAssets[0].DurationSeconds)
	})

	t.Run("merchant_update_rejects_unknown_final_asset_key", func(t *testing.T) {
		unknownAssetProductResults, err := store.CreateProductTx(ctx, db.CreateProductTxParams{
			OrganizationID: org.ID,
			CategoryID:     category.ID,
			Name:           "Unknown Final Asset Product",
			Slug:           "unknown-final-asset-" + uuid.New().String()[:8],
			Description:    []byte(`{"text":"unknown final asset"}`),
			Specification:  []byte(`{"weight":"1kg"}`),
			Variants: []db.ProductVariantParams{
				{
					Sku:               "SKU-UNKNOWN-FINAL-ASSET-" + uuid.New().String()[:6],
					Name:              "Unknown Final Asset Variant",
					Price:             decimal.NewFromFloat(29.99),
					AttributeValueIDs: []int64{val.ID},
				},
			},
		})
		require.NoError(t, err)

		unknownFinalKey := fmt.Sprintf(
			"assets/%s/%s/not-on-this-product-%s.png",
			org.ID.String(),
			currentYearMonth,
			uuid.New().String()[:8],
		)
		bodyBytes, err := json.Marshal(CreateProductRequest{
			Name:          "Unknown Final Asset Product Updated",
			Slug:          "unknown-final-asset-updated-" + uuid.New().String()[:8],
			CategoryID:    category.ID,
			Description:   json.RawMessage(`{"text":"unknown final asset updated"}`),
			Specification: json.RawMessage(`{"weight":"1kg"}`),
			Assets: []ProductAssetRequest{
				{
					Token:     unknownFinalKey,
					IsPrimary: true,
					SortOrder: 1,
				},
			},
			Variants: []VariantRequest{
				{
					Sku:               unknownAssetProductResults.ProductVariants[0].Sku,
					Name:              "Unknown Final Asset Variant Updated",
					Price:             decimal.NewFromFloat(34.99),
					AttributeValueIDs: []int64{val.ID},
				},
			},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/v1/products/"+unknownAssetProductResults.Product.ID.String(),
			bytes.NewReader(bodyBytes),
		)
		req = req.WithContext(
			context.WithValue(req.Context(), middleware.OrganizationContextKey{}, org),
		)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", unknownAssetProductResults.Product.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		err = h.UpdateProduct(rec, req)
		var apiErr apierror.APIError
		require.True(t, errors.As(err, &apiErr), "expected APIError, got: %v", err)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	})

	t.Run("merchant_update_replaces_asset_and_cleans_up_old_and_temp_objects", func(t *testing.T) {
		oldFinalKey := fmt.Sprintf(
			"assets/%s/%s/old-asset-%s.png",
			org.ID.String(),
			currentYearMonth,
			uuid.New().String()[:8],
		)
		_, err = s3Storage.S3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(oldFinalKey),
			Body:   bytes.NewReader([]byte("old image bytes")),
		})
		require.NoError(t, err)

		assetProductResults, err := store.CreateProductTx(ctx, db.CreateProductTxParams{
			OrganizationID: org.ID,
			CategoryID:     category.ID,
			Name:           "Asset Replacement Product",
			Slug:           "asset-replace-" + uuid.New().String()[:8],
			Description:    []byte(`{"text":"asset replace"}`),
			Specification:  []byte(`{"weight":"1kg"}`),
			Assets: []db.ProductAssetParams{
				{
					AssetKey:  oldFinalKey,
					Type:      string(util.ProductAssetImage),
					MimeType:  "image/png",
					SortOrder: 1,
					IsPrimary: true,
				},
			},
			Variants: []db.ProductVariantParams{
				{
					Sku:               "SKU-ASSET-REPLACE-" + uuid.New().String()[:6],
					Name:              "Asset Replacement Variant",
					Price:             decimal.NewFromFloat(39.99),
					AttributeValueIDs: []int64{val.ID},
				},
			},
		})
		require.NoError(t, err)

		newToken := uuid.New().String()
		newTempKey := fmt.Sprintf("temp/%s.png", newToken)
		newFinalKey := fmt.Sprintf(
			"assets/%s/%s/new-asset-%s.png",
			org.ID.String(),
			currentYearMonth,
			newToken,
		)
		_, err = s3Storage.S3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(newTempKey),
			Body:   bytes.NewReader([]byte("new image bytes")),
		})
		require.NoError(t, err)

		pending := PendingUpload{
			Token:          newToken,
			TempKey:        newTempKey,
			FinalKey:       newFinalKey,
			Type:           string(util.ProductAssetImage),
			ContentType:    "image/png",
			OriginalName:   "new.png",
			OrganizationID: org.ID,
		}
		cacheBytes, err := json.Marshal(pending)
		require.NoError(t, err)
		err = redisClient.CachePendingUpload(ctx, newToken, cacheBytes)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = s3Storage.DeleteObject(context.Background(), bucketName, oldFinalKey)
			_ = s3Storage.DeleteObject(context.Background(), bucketName, newTempKey)
			_ = s3Storage.DeleteObject(context.Background(), bucketName, newFinalKey)
		})

		bodyBytes, err := json.Marshal(CreateProductRequest{
			Name:          "Asset Replacement Product Updated",
			Slug:          "asset-replace-updated-" + uuid.New().String()[:8],
			CategoryID:    category.ID,
			Description:   json.RawMessage(`{"text":"asset replace updated"}`),
			Specification: json.RawMessage(`{"weight":"1kg"}`),
			Assets: []ProductAssetRequest{
				{
					Token:     newToken,
					IsPrimary: true,
					SortOrder: 1,
				},
			},
			Variants: []VariantRequest{
				{
					Sku:               assetProductResults.ProductVariants[0].Sku,
					Name:              "Asset Replacement Variant Updated",
					Price:             decimal.NewFromFloat(44.99),
					AttributeValueIDs: []int64{val.ID},
				},
			},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPut,
			"/v1/products/"+assetProductResults.Product.ID.String(),
			bytes.NewReader(bodyBytes),
		)
		req = req.WithContext(
			context.WithValue(req.Context(), middleware.OrganizationContextKey{}, org),
		)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", assetProductResults.Product.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		err = h.UpdateProduct(rec, req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rec.Code)

		_, err = s3Storage.S3.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(newFinalKey),
		})
		require.NoError(t, err, "new final asset should exist")

		require.Eventually(t, func() bool {
			_, oldErr := s3Storage.S3.HeadObject(ctx, &s3.HeadObjectInput{
				Bucket: aws.String(bucketName),
				Key:    aws.String(oldFinalKey),
			})
			_, tempErr := s3Storage.S3.HeadObject(ctx, &s3.HeadObjectInput{
				Bucket: aws.String(bucketName),
				Key:    aws.String(newTempKey),
			})
			_, redisErr := redisClient.GetPendingUpload(ctx, newToken)
			return oldErr != nil && tempErr != nil && redisErr != nil
		}, 2*time.Second, 10*time.Millisecond, "old final asset, temp asset, and pending token should be cleaned up")
	})

	t.Run("merchant_delete_product", func(t *testing.T) {
		assetKey := fmt.Sprintf("assets/%s/%s/delete-me.png", org.ID.String(), currentYearMonth)
		_, err = s3Storage.S3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(assetKey),
			Body:   bytes.NewReader([]byte("asset to delete")),
		})
		require.NoError(t, err)

		_, err = store.CreateProductAsset(ctx, db.CreateProductAssetParams{
			ProductID: productID,
			AssetKey:  assetKey,
			Type:      "image",
			MimeType:  "image/png",
			IsPrimary: true,
			AltText:   nil,
			SortOrder: 1,
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodDelete, "/v1/products/"+productID.String(), nil)
		reqCtx := context.WithValue(req.Context(), middleware.OrganizationContextKey{}, org)
		req = req.WithContext(reqCtx)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", productID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		err = h.DeleteProduct(rec, req)
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, rec.Code)

		_, err = store.GetProductByID(ctx, productID)
		require.ErrorIs(t, err, db.ErrNotFound)

		// Verify S3 asset is deleted in background
		require.Eventually(t, func() bool {
			_, headErr := s3Storage.S3.HeadObject(ctx, &s3.HeadObjectInput{
				Bucket: aws.String(bucketName),
				Key:    aws.String(assetKey),
			})
			return headErr != nil
		}, 2*time.Second, 10*time.Millisecond, "S3 asset should be deleted in background after product deletion")
	})
}
