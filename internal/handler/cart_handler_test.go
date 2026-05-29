//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
	"github.com/skynicklaus/ecommerce-api/internal/middleware"
	"github.com/skynicklaus/ecommerce-api/internal/password"
	"github.com/skynicklaus/ecommerce-api/internal/storage"
	"github.com/skynicklaus/ecommerce-api/util"
)

type cartHandlerFixture struct {
	store         db.Store
	pool          *pgxpool.Pool
	router        *chi.Mux
	customerToken string
	merchantToken string
	buyerOrg      db.Organization
	sellerOrg     db.Organization
	variant       db.ProductVariant
}

func TestCartHandlers_Integration(t *testing.T) {
	fixture := newCartHandlerFixture(t)

	t.Run("missing_auth_returns_401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/cart", nil)
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("merchant_token_is_rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/cart", nil)
		req.Header.Set("Authorization", "Bearer "+fixture.merchantToken)
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("get_empty_cart", func(t *testing.T) {
		req := newCartJSONRequest(t, http.MethodGet, "/v1/cart", fixture.customerToken, nil)
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		var resp CartResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Equal(t, fixture.buyerOrg.ID, resp.BuyerOrgID)
		require.Empty(t, resp.Groups)
	})

	t.Run("get_existing_cart_does_not_write", func(t *testing.T) {
		_ = getCartForTest(t, fixture)
		before, err := fixture.store.GetCartByBuyerOrgID(t.Context(), fixture.buyerOrg.ID)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		req := newCartJSONRequest(t, http.MethodGet, "/v1/cart", fixture.customerToken, nil)
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		after, err := fixture.store.GetCartByBuyerOrgID(t.Context(), fixture.buyerOrg.ID)
		require.NoError(t, err)
		require.Equal(t, before.UpdatedAt, after.UpdatedAt)
	})

	var cartItemID uuid.UUID
	var shopGroupID uuid.UUID

	t.Run("add_item_success", func(t *testing.T) {
		req := newCartJSONRequest(t, http.MethodPost, "/v1/cart/items", fixture.customerToken, AddCartItemRequest{
			ProductVariantID: fixture.variant.ID,
			Quantity:         2,
		})
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusCreated, rr.Code)

		var resp CartItemMutationResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.NotZero(t, resp.Item.ID)
		require.Equal(t, fixture.variant.ID, resp.Item.ProductVariantID)
		require.Equal(t, int16(2), resp.Item.Quantity)
		require.True(t, resp.Item.IsSelected)
		cartItemID = resp.Item.ID

		cartResp := getCartForTest(t, fixture)
		require.Len(t, cartResp.Groups, 1)
		require.Len(t, cartResp.Groups[0].Items, 1)
		require.True(t, decimal.NewFromInt(50).Equal(cartResp.Subtotal))
		require.True(t, decimal.NewFromInt(50).Equal(cartResp.SelectedSubtotal))
		require.Equal(t, int32(2), cartResp.TotalQuantity)
		require.Equal(t, int32(2), cartResp.SelectedQuantity)
		require.True(t, decimal.NewFromInt(50).Equal(cartResp.Groups[0].Subtotal))
		require.True(t, decimal.NewFromInt(50).Equal(cartResp.Groups[0].SelectedSubtotal))
		require.Equal(t, int32(2), cartResp.Groups[0].TotalQuantity)
		require.Equal(t, int32(2), cartResp.Groups[0].SelectedQuantity)
		require.True(t, decimal.NewFromInt(50).Equal(cartResp.Groups[0].Items[0].Subtotal))
		shopGroupID = cartResp.Groups[0].ID
		require.Equal(t, fixture.sellerOrg.ID, cartResp.Groups[0].MerchantOrgID)
		require.NotNil(t, cartResp.Groups[0].Items[0].ThumbnailURL)
		require.NotEmpty(t, *cartResp.Groups[0].Items[0].ThumbnailURL)
		require.NotNil(t, cartResp.Groups[0].Items[0].ThumbnailSource)
		require.Equal(t, "product", *cartResp.Groups[0].Items[0].ThumbnailSource)
	})

	t.Run("get_cart_prefers_variant_thumbnail_over_product_thumbnail", func(t *testing.T) {
		variantID := fixture.variant.ID
		_, err := fixture.store.CreateProductAsset(t.Context(), db.CreateProductAssetParams{
			ProductID:        fixture.variant.ProductID,
			ProductVariantID: &variantID,
			AssetKey:         "assets/cart-handler/variant-" + uuid.NewString() + ".webp",
			Type:             string(util.ProductAssetImage),
			MimeType:         "image/webp",
			AltText:          nil,
			SortOrder:        10,
			IsPrimary:        false,
		})
		require.NoError(t, err)

		cartResp := getCartForTest(t, fixture)
		require.Len(t, cartResp.Groups, 1)
		require.Len(t, cartResp.Groups[0].Items, 1)
		require.NotNil(t, cartResp.Groups[0].Items[0].ThumbnailURL)
		require.NotEmpty(t, *cartResp.Groups[0].Items[0].ThumbnailURL)
		require.NotNil(t, cartResp.Groups[0].Items[0].ThumbnailSource)
		require.Equal(t, "variant", *cartResp.Groups[0].Items[0].ThumbnailSource)
	})

	t.Run("add_item_validation_error", func(t *testing.T) {
		req := newCartJSONRequest(t, http.MethodPost, "/v1/cart/items", fixture.customerToken, AddCartItemRequest{
			ProductVariantID: fixture.variant.ID,
			Quantity:         0,
		})
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
	})

	t.Run("add_item_inactive_variant_returns_404", func(t *testing.T) {
		inactiveVariant := createCartHandlerVariant(t, fixture.store, fixture.pool, fixture.sellerOrg, false)
		req := newCartJSONRequest(t, http.MethodPost, "/v1/cart/items", fixture.customerToken, AddCartItemRequest{
			ProductVariantID: inactiveVariant.ID,
			Quantity:         1,
		})
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("update_quantity_success", func(t *testing.T) {
		req := newCartJSONRequest(
			t,
			http.MethodPatch,
			"/v1/cart/items/"+cartItemID.String(),
			fixture.customerToken,
			UpdateCartItemQuantityRequest{Quantity: 5},
		)
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		var resp CartItemMutationResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Equal(t, int16(5), resp.Item.Quantity)
	})

	t.Run("update_quantity_invalid_uuid", func(t *testing.T) {
		req := newCartJSONRequest(
			t,
			http.MethodPatch,
			"/v1/cart/items/not-a-uuid",
			fixture.customerToken,
			UpdateCartItemQuantityRequest{Quantity: 1},
		)
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("update_quantity_not_found", func(t *testing.T) {
		req := newCartJSONRequest(
			t,
			http.MethodPatch,
			"/v1/cart/items/"+uuid.NewString(),
			fixture.customerToken,
			UpdateCartItemQuantityRequest{Quantity: 1},
		)
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("set_item_selected_success", func(t *testing.T) {
		selected := false
		req := newCartJSONRequest(
			t,
			http.MethodPatch,
			"/v1/cart/items/"+cartItemID.String()+"/selected",
			fixture.customerToken,
			SetSelectedRequest{IsSelected: &selected},
		)
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		var resp CartItemMutationResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.False(t, resp.Item.IsSelected)
	})

	t.Run("set_group_selected_success", func(t *testing.T) {
		selected := false
		req := newCartJSONRequest(
			t,
			http.MethodPatch,
			"/v1/cart/shop-groups/"+shopGroupID.String()+"/selected",
			fixture.customerToken,
			SetSelectedRequest{IsSelected: &selected},
		)
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		var mutationResp CartShopGroupMutationResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &mutationResp))
		require.Equal(t, shopGroupID, mutationResp.ShopGroup.ID)
		require.Equal(t, fixture.sellerOrg.Name, mutationResp.ShopGroup.MerchantName)
		require.False(t, mutationResp.ShopGroup.IsSelected)
		require.Len(t, mutationResp.ShopGroup.Items, 1)
		require.True(t, decimal.NewFromInt(125).Equal(mutationResp.ShopGroup.Subtotal))
		require.True(t, decimal.Zero.Equal(mutationResp.ShopGroup.SelectedSubtotal))
		require.Equal(t, int32(5), mutationResp.ShopGroup.TotalQuantity)
		require.Equal(t, int32(0), mutationResp.ShopGroup.SelectedQuantity)

		cartResp := getCartForTest(t, fixture)
		require.Len(t, cartResp.Groups, 1)
		require.False(t, cartResp.Groups[0].IsSelected)
		require.False(t, cartResp.Groups[0].Items[0].IsSelected)
		require.True(t, decimal.NewFromInt(125).Equal(cartResp.Subtotal))
		require.True(t, decimal.Zero.Equal(cartResp.SelectedSubtotal))
		require.Equal(t, int32(5), cartResp.TotalQuantity)
		require.Equal(t, int32(0), cartResp.SelectedQuantity)
		require.True(t, decimal.NewFromInt(125).Equal(cartResp.Groups[0].Subtotal))
		require.True(t, decimal.Zero.Equal(cartResp.Groups[0].SelectedSubtotal))
	})

	t.Run("delete_item_success_then_not_found", func(t *testing.T) {
		req := newCartJSONRequest(t, http.MethodDelete, "/v1/cart/items/"+cartItemID.String(), fixture.customerToken, nil)
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusNoContent, rr.Code)

		cartResp := getCartForTest(t, fixture)
		require.Empty(t, cartResp.Groups)

		req = newCartJSONRequest(t, http.MethodDelete, "/v1/cart/items/"+cartItemID.String(), fixture.customerToken, nil)
		rr = httptest.NewRecorder()
		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusNotFound, rr.Code)
	})
}

func TestCartHandlers_InactiveItemMutationResponses(t *testing.T) {
	t.Run("update_quantity_returns_inactive_variant_details_after_mutation", func(t *testing.T) {
		fixture := newCartHandlerFixture(t)
		cartItemID := addCartItemForTest(t, fixture, fixture.variant.ID, 1)

		_, err := fixture.pool.Exec(t.Context(), "UPDATE product_variants SET is_active = FALSE WHERE id = $1", fixture.variant.ID)
		require.NoError(t, err)

		req := newCartJSONRequest(
			t,
			http.MethodPatch,
			"/v1/cart/items/"+cartItemID.String(),
			fixture.customerToken,
			UpdateCartItemQuantityRequest{Quantity: 3},
		)
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		var resp CartItemMutationResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Equal(t, cartItemID, resp.Item.ID)
		require.Equal(t, int16(3), resp.Item.Quantity)
		require.False(t, resp.Item.VariantIsActive)
		require.Equal(t, string(util.ProductStatusActive), resp.Item.ProductStatus)
	})

	t.Run("set_selected_returns_non_active_product_details_after_mutation", func(t *testing.T) {
		fixture := newCartHandlerFixture(t)
		cartItemID := addCartItemForTest(t, fixture, fixture.variant.ID, 1)

		_, err := fixture.pool.Exec(
			t.Context(),
			"UPDATE products SET status = 'draft' WHERE id = $1",
			fixture.variant.ProductID,
		)
		require.NoError(t, err)

		selected := false
		req := newCartJSONRequest(
			t,
			http.MethodPatch,
			"/v1/cart/items/"+cartItemID.String()+"/selected",
			fixture.customerToken,
			SetSelectedRequest{IsSelected: &selected},
		)
		rr := httptest.NewRecorder()

		fixture.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		var resp CartItemMutationResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Equal(t, cartItemID, resp.Item.ID)
		require.False(t, resp.Item.IsSelected)
		require.True(t, resp.Item.VariantIsActive)
		require.Equal(t, string(util.ProductStatusDraft), resp.Item.ProductStatus)
	})
}

func newCartHandlerFixture(t *testing.T) cartHandlerFixture {
	t.Helper()
	ctx := t.Context()

	dbSource := os.Getenv("DB_SOURCE")
	if dbSource == "" {
		t.Skip("DB_SOURCE not set")
	}

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
	t.Cleanup(func() { _ = redisClient.Close() })
	s3Storage, err := storage.New(ctx)
	require.NoError(t, err)

	h := NewV1Handler(store, logger, redisClient, s3Storage)
	midware := middleware.New(store, redisClient)

	r := chi.NewRouter()
	r.Post("/v1/auth/customer/login", makeTestHandler(h.LoginCustomer))
	r.Post("/v1/auth/merchant/login", makeTestHandler(h.LoginMerchant))
	r.Group(func(r chi.Router) {
		r.Use(midware.RequireService(util.SessionServiceBuyerPlatform))
		r.Get("/v1/cart", makeTestHandler(h.GetCart))
		r.Post("/v1/cart/items", makeTestHandler(h.AddCartItem))
		r.Patch("/v1/cart/items/{id}", makeTestHandler(h.UpdateCartItemQuantity))
		r.Patch("/v1/cart/items/{id}/selected", makeTestHandler(h.SetCartItemSelected))
		r.Delete("/v1/cart/items/{id}", makeTestHandler(h.RemoveCartItem))
		r.Patch("/v1/cart/shop-groups/{id}/selected", makeTestHandler(h.SetCartShopGroupSelected))
		r.Post("/v1/checkout", makeTestHandler(h.CreateCheckout))
		r.Post("/v1/checkout/{id}/cancel", makeTestHandler(h.CancelCheckout))
		r.Post("/v1/payments/{id}/confirm", makeTestHandler(h.ConfirmManualPayment))
	})

	buyerOrg := createCartHandlerOrganization(
		t,
		store,
		connPool,
		util.OrganizationTypeIndividual,
		util.OrganizationCapabilityBuyer,
	)
	sellerOrg := createCartHandlerOrganization(
		t,
		store,
		connPool,
		util.OrganizationTypeMerchant,
		util.OrganizationCapabilitySeller,
	)
	customerEmail, customerPassword := createCartHandlerCustomer(t, store, connPool, buyerOrg)
	merchantEmail, merchantPassword := createCartHandlerMerchant(t, store, connPool, sellerOrg)
	variant := createCartHandlerVariant(t, store, connPool, sellerOrg, true)

	return cartHandlerFixture{
		store:         store,
		pool:          connPool,
		router:        r,
		customerToken: loginCartHandlerUser(t, r, "/v1/auth/customer/login", customerEmail, customerPassword),
		merchantToken: loginCartHandlerUser(t, r, "/v1/auth/merchant/login", merchantEmail, merchantPassword),
		buyerOrg:      buyerOrg,
		sellerOrg:     sellerOrg,
		variant:       variant,
	}
}

func createCartHandlerOrganization(
	t *testing.T,
	store db.Store,
	pool *pgxpool.Pool,
	organizationType util.OrganizationType,
	capability util.OrganizationCapability,
) db.Organization {
	t.Helper()

	org, err := store.CreateOrganization(t.Context(), db.CreateOrganizationParams{
		Name:       string(organizationType) + " cart handler " + uuid.NewString(),
		Slug:       string(organizationType) + "-cart-handler-" + uuid.NewString(),
		Status:     string(util.OrganizationStatusActive),
		Type:       string(organizationType),
		Capability: string(capability),
		Metadata:   []byte("{}"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
	})
	return org
}

func createCartHandlerCustomer(
	t *testing.T,
	store db.Store,
	pool *pgxpool.Pool,
	buyerOrg db.Organization,
) (string, string) {
	t.Helper()

	identity, err := store.CreateIdentity(t.Context(), string(util.IdentityCustomer))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM identities WHERE id = $1", identity.ID)
	})

	email := "cart-buyer-" + uuid.NewString()[:8] + "@test.com"
	pass := "supersecure123"
	hashedPass, err := password.HashPassword(pass)
	require.NoError(t, err)
	customer, err := store.CreateCustomer(t.Context(), db.CreateCustomerParams{
		IdentityID: identity.ID,
		Name:       "Cart Buyer",
		Email:      email,
	})
	require.NoError(t, err)
	_, err = store.CreateCustomerAccount(t.Context(), db.CreateCustomerAccountParams{
		CustomerID:            customer.ID,
		AccountID:             "credential-" + uuid.NewString()[:8],
		ProviderID:            string(util.ProviderIDCredential),
		AccessToken:           nil,
		RefreshToken:          nil,
		AccessTokenExpiresAt:  nil,
		RefreshTokenExpiresAt: nil,
		Scope:                 nil,
		IDToken:               nil,
		HashedPassword:        &hashedPass,
	})
	require.NoError(t, err)
	_, err = store.CreateMember(t.Context(), db.CreateMemberParams{
		IdentityID:     identity.ID,
		OrganizationID: buyerOrg.ID,
	})
	require.NoError(t, err)

	return email, pass
}

func createCartHandlerMerchant(
	t *testing.T,
	store db.Store,
	pool *pgxpool.Pool,
	sellerOrg db.Organization,
) (string, string) {
	t.Helper()

	identity, err := store.CreateIdentity(t.Context(), string(util.IdentityUser))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM identities WHERE id = $1", identity.ID)
	})

	email := "cart-merchant-" + uuid.NewString()[:8] + "@test.com"
	pass := "supersecure123"
	hashedPass, err := password.HashPassword(pass)
	require.NoError(t, err)
	user, err := store.CreateUser(t.Context(), db.CreateUserParams{
		IdentityID: identity.ID,
		Name:       "Cart Merchant",
		Email:      email,
	})
	require.NoError(t, err)
	_, err = store.CreateUserAccount(t.Context(), db.CreateUserAccountParams{
		UserID:                user.ID,
		AccountID:             "credential-" + uuid.NewString()[:8],
		ProviderID:            string(util.ProviderIDCredential),
		AccessToken:           nil,
		RefreshToken:          nil,
		AccessTokenExpiresAt:  nil,
		RefreshTokenExpiresAt: nil,
		Scope:                 nil,
		IDToken:               nil,
		HashedPassword:        &hashedPass,
	})
	require.NoError(t, err)
	_, err = store.CreateMember(t.Context(), db.CreateMemberParams{
		IdentityID:     identity.ID,
		OrganizationID: sellerOrg.ID,
	})
	require.NoError(t, err)

	return email, pass
}

func createCartHandlerVariant(
	t *testing.T,
	store db.Store,
	pool *pgxpool.Pool,
	sellerOrg db.Organization,
	active bool,
) db.ProductVariant {
	t.Helper()

	orgID := sellerOrg.ID
	category, err := store.CreateCategory(t.Context(), db.CreateCategoryParams{
		OrganizationID: &orgID,
		Name:           "Cart Category " + uuid.NewString(),
		Slug:           "cart-category-" + uuid.NewString(),
		SortOrder:      1,
	})
	require.NoError(t, err)
	product, err := store.CreateProduct(t.Context(), db.CreateProductParams{
		OrganizationID: sellerOrg.ID,
		CategoryID:     category.ID,
		Name:           "Cart Product " + uuid.NewString(),
		Slug:           "cart-product-" + uuid.NewString(),
		Description:    []byte(`{"text":"cart product"}`),
		Specification:  []byte(`{}`),
	})
	require.NoError(t, err)
	if active {
		_, err = store.UpdateProductStatus(t.Context(), db.UpdateProductStatusParams{
			ID:             product.ID,
			OrganizationID: sellerOrg.ID,
			Status:         string(util.ProductStatusActive),
		})
		require.NoError(t, err)
	}
	_, err = store.CreateProductAsset(t.Context(), db.CreateProductAssetParams{
		ProductID: product.ID,
		AssetKey:  "assets/cart-handler/product-" + uuid.NewString() + ".webp",
		Type:      string(util.ProductAssetImage),
		MimeType:  "image/webp",
		AltText:   nil,
		SortOrder: 0,
		IsPrimary: true,
	})
	require.NoError(t, err)

	variant, err := store.CreateProductVariant(t.Context(), db.CreateProductVariantParams{
		ProductID:      product.ID,
		OrganizationID: sellerOrg.ID,
		Sku:            "cart-sku-" + uuid.NewString(),
		Name:           "Cart Variant",
		Price:          decimal.NewFromInt(25),
	})
	require.NoError(t, err)
	if active {
		_, err = pool.Exec(t.Context(), "UPDATE product_variants SET is_active = TRUE WHERE id = $1", variant.ID)
		require.NoError(t, err)
		variant.IsActive = true
	}
	return variant
}

func loginCartHandlerUser(t *testing.T, r http.Handler, path, email, pass string) string {
	t.Helper()

	body, err := json.Marshal(LoginRequest{Email: email, Password: pass})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp LoginResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Token)
	return resp.Token
}

func newCartJSONRequest(t *testing.T, method, path, token string, body any) *http.Request {
	t.Helper()

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func addCartItemForTest(
	t *testing.T,
	fixture cartHandlerFixture,
	variantID uuid.UUID,
	quantity int16,
) uuid.UUID {
	t.Helper()

	req := newCartJSONRequest(t, http.MethodPost, "/v1/cart/items", fixture.customerToken, AddCartItemRequest{
		ProductVariantID: variantID,
		Quantity:         quantity,
	})
	rr := httptest.NewRecorder()
	fixture.router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	var resp CartItemMutationResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	return resp.Item.ID
}

func getCartForTest(t *testing.T, fixture cartHandlerFixture) CartResponse {
	t.Helper()

	req := newCartJSONRequest(t, http.MethodGet, "/v1/cart", fixture.customerToken, nil)
	rr := httptest.NewRecorder()
	fixture.router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp CartResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	return resp
}
