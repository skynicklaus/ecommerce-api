//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
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
		dbSource = "postgresql://app_system:system_secret@localhost:5432/ecommerce?sslmode=disable"
	}

	bucketName := os.Getenv("S3_BUCKET")
	if bucketName == "" {
		bucketName = "ecommerce-assets"
	}

	// 2. Initialize real infrastructure clients
	connPool, err := pgxpool.New(ctx, dbSource)
	require.NoError(t, err)
	defer connPool.Close()

	store := db.NewStore(connPool)
	logger := util.NewLogger()

	redisClient := cache.NewRedis(store, logger)
	defer redisClient.Close()

	s3Storage, err := storage.New(ctx)
	require.NoError(t, err)

	// 3. Create the Handler instance
	h := NewV1Handler(store, logger, redisClient, s3Storage).(*V1Handler)

	// Override config params to ensure local development targets are fully matched
	h.bucket = &bucketName

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
		finalKey := fmt.Sprintf("assets/%s/2026/05/%s.png", org.ID.String(), token)

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
			Assets: []ProductAssetReq{
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

		// Wait briefly for fire-and-forget background S3/Redis deletions to execute
		time.Sleep(100 * time.Millisecond)

		// F. Verify S3 permanent object exists, and S3 temporary object is deleted
		_, err = s3Storage.S3.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(finalKey),
		})
		require.NoError(t, err, "S3 permanent asset should exist")

		_, err = s3Storage.S3.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(tempKey),
		})
		require.Error(t, err, "S3 temporary asset should have been deleted")

		// G. Verify Redis cache token is deleted
		_, err = redisClient.GetPendingUpload(ctx, token)
		require.Error(t, err, "Redis pending upload token should be deleted")
	})

	t.Run("failure_triggers_s3_copy_rollback", func(t *testing.T) {
		token := uuid.New().String()
		tempKey := fmt.Sprintf("temp/%s.png", token)
		finalKey := fmt.Sprintf("assets/%s/2026/05/%s.png", org.ID.String(), token)

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
			Assets: []ProductAssetReq{
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

		// D. Invoke Handler - should error and return database constraints failure
		err = h.CreateProduct(rec, req)
		require.Error(t, err, "Should fail on category foreign key constraint")

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
		finalKey := fmt.Sprintf("assets/%s/2026/05/%s.png", org.ID.String(), token)

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
			Assets: []ProductAssetReq{
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

		// C. Concurrent request lock check - Mock setting locked key in Redis, should fail with 409 Conflict
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
		testKey := "assets/" + org.ID.String() + "/2026/05/test-asset.png"

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

		// Set chi URLParam to simulate slug_or_id routing matching
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("slug_or_id", createdProductID.String())
		reqID = reqID.WithContext(context.WithValue(reqID.Context(), chi.RouteCtxKey, rctx))

		recID := httptest.NewRecorder()
		err = h.GetProductDetails(recID, reqID)
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
		rctxSlug.URLParams.Add("slug_or_id", createdProductSlug)
		reqSlug = reqSlug.WithContext(
			context.WithValue(reqSlug.Context(), chi.RouteCtxKey, rctxSlug),
		)

		recSlug := httptest.NewRecorder()
		err = h.GetProductDetails(recSlug, reqSlug)
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
		rctxInv.URLParams.Add("slug_or_id", "invalid-slug-xyz")
		reqInvalid = reqInvalid.WithContext(
			context.WithValue(reqInvalid.Context(), chi.RouteCtxKey, rctxInv),
		)

		recInvalid := httptest.NewRecorder()
		err = h.GetProductDetails(recInvalid, reqInvalid)
		require.Error(t, err, "Should error on non-existent product")

		// 4. List Products for Organization
		reqList := httptest.NewRequest(http.MethodGet, "/v1/products", nil)
		reqListCtx := context.WithValue(reqList.Context(), middleware.OrganizationContextKey{}, org)
		reqList = reqList.WithContext(reqListCtx)

		recList := httptest.NewRecorder()
		err = h.ListProducts(recList, reqList)
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
