package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/internal/middleware"
	"github.com/skynicklaus/ecommerce-api/util"
)

type AddCartItemRequest struct {
	ProductVariantID uuid.UUID `json:"productVariantId" validate:"required"`
	Quantity         int16     `json:"quantity"         validate:"required,gt=0"`
}

type UpdateCartItemQuantityRequest struct {
	Quantity int16 `json:"quantity" validate:"required,gt=0"`
}

type SetSelectedRequest struct {
	IsSelected *bool `json:"isSelected" validate:"required"`
}

type CartItemResponse struct {
	ID               uuid.UUID       `json:"id"`
	ProductVariantID uuid.UUID       `json:"productVariantId"`
	ProductID        uuid.UUID       `json:"productId"`
	ProductName      string          `json:"productName"`
	ProductSlug      string          `json:"productSlug"`
	VariantName      string          `json:"variantName"`
	Sku              string          `json:"sku"`
	Quantity         int16           `json:"quantity"`
	UnitPrice        decimal.Decimal `json:"unitPrice"`
	Subtotal         decimal.Decimal `json:"subtotal"`
	CurrentPrice     decimal.Decimal `json:"currentPrice"`
	IsSelected       bool            `json:"isSelected"`
	VariantIsActive  bool            `json:"variantIsActive"`
	ProductStatus    string          `json:"productStatus"`
	ThumbnailURL     *string         `json:"thumbnailUrl,omitempty"`
	ThumbnailSource  *string         `json:"thumbnailSource,omitempty"`
}

type CartShopGroupResponse struct {
	ID               uuid.UUID          `json:"id"`
	MerchantOrgID    uuid.UUID          `json:"merchantOrgId"`
	MerchantName     string             `json:"merchantName"`
	IsSelected       bool               `json:"isSelected"`
	Subtotal         decimal.Decimal    `json:"subtotal"`
	SelectedSubtotal decimal.Decimal    `json:"selectedSubtotal"`
	TotalQuantity    int32              `json:"totalQuantity"`
	SelectedQuantity int32              `json:"selectedQuantity"`
	Items            []CartItemResponse `json:"items"`
}

type CartResponse struct {
	ID               uuid.UUID               `json:"id"`
	BuyerOrgID       uuid.UUID               `json:"buyerOrgId"`
	Subtotal         decimal.Decimal         `json:"subtotal"`
	SelectedSubtotal decimal.Decimal         `json:"selectedSubtotal"`
	TotalQuantity    int32                   `json:"totalQuantity"`
	SelectedQuantity int32                   `json:"selectedQuantity"`
	Groups           []CartShopGroupResponse `json:"groups"`
}

type CartItemMutationResponse struct {
	Item CartItemResponse `json:"item"`
}

type CartShopGroupMutationResponse struct {
	ShopGroup CartShopGroupResponse `json:"shopGroup"`
}

// GetCart godoc
//
//	@Summary		Get cart
//	@Description	Returns the authenticated buyer organization's cart.
//	@Tags			cart
//	@Produce		json
//	@Success		200	{object}	CartResponse
//	@Failure		401	{object}	apierror.APIError
//	@Failure		403	{object}	apierror.APIError
//	@Failure		500	{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/cart [get]
func (h *V1Handler) GetCart(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	buyerOrgID, err := buyerOrgIDFromCtx(ctx)
	if err != nil {
		return err
	}

	cart, err := h.store.GetCartByBuyerOrgID(ctx, buyerOrgID)
	if err != nil {
		if !errors.Is(err, db.ErrNotFound) {
			return fmt.Errorf("failed to get cart: %w", err)
		}
		cart, err = h.store.CreateCart(ctx, buyerOrgID)
		if err != nil {
			return mapCartWriteError(err)
		}
	}

	details, err := h.store.GetCartDetails(ctx, buyerOrgID)
	if err != nil {
		return fmt.Errorf("failed to get cart details: %w", err)
	}

	resp := h.buildCartResponse(ctx, cart, details)

	return WriteJSON(w, http.StatusOK, resp)
}

// AddCartItem godoc
//
//	@Summary		Add item to cart
//	@Description	Adds an active product variant to the authenticated buyer organization's cart.
//	@Tags			cart
//	@Accept			json
//	@Produce		json
//	@Param			request	body		AddCartItemRequest	true	"Cart item payload"
//	@Success		201		{object}	CartItemMutationResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		403		{object}	apierror.APIError
//	@Failure		404		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/cart/items [post]
func (h *V1Handler) AddCartItem(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	buyerOrgID, err := buyerOrgIDFromCtx(ctx)
	if err != nil {
		return err
	}

	var req AddCartItemRequest
	if err = decodeJSON(w, r, &req); err != nil {
		return err
	}
	if err = h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	result, err := h.store.AddCartItemTx(ctx, db.AddCartItemTxParams{
		BuyerOrgID:       buyerOrgID,
		ProductVariantID: req.ProductVariantID,
		Quantity:         req.Quantity,
	})
	if err != nil {
		return mapCartWriteError(err)
	}

	return WriteJSON(w, http.StatusCreated, CartItemMutationResponse{
		Item: cartItemResponseFromTx(result.Item, result.Variant),
	})
}

// UpdateCartItemQuantity godoc
//
//	@Summary		Update cart item quantity
//	@Description	Updates a cart item's quantity for the authenticated buyer organization.
//	@Tags			cart
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string							true	"Cart item UUID"
//	@Param			request	body		UpdateCartItemQuantityRequest	true	"Quantity payload"
//	@Success		200		{object}	CartItemMutationResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		403		{object}	apierror.APIError
//	@Failure		404		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/cart/items/{id} [patch]
func (h *V1Handler) UpdateCartItemQuantity(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	buyerOrgID, err := buyerOrgIDFromCtx(ctx)
	if err != nil {
		return err
	}
	cartItemID, err := parseIDParam(r, "invalid cart item id")
	if err != nil {
		return err
	}

	var req UpdateCartItemQuantityRequest
	if err = decodeJSON(w, r, &req); err != nil {
		return err
	}
	if err = h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	result, err := h.store.UpdateCartItemQuantityTx(ctx, db.UpdateCartItemQuantityTxParams{
		BuyerOrgID: buyerOrgID,
		CartItemID: cartItemID,
		Quantity:   req.Quantity,
	})
	if err != nil {
		return mapCartWriteError(err)
	}

	detail, err := h.store.GetCartItemDetailsForBuyerOrg(ctx, db.GetCartItemDetailsForBuyerOrgParams{
		CartItemID: result.Item.ID,
		BuyerOrgID: buyerOrgID,
	})
	if err != nil {
		return mapCartWriteError(err)
	}

	return WriteJSON(w, http.StatusOK, CartItemMutationResponse{
		Item: cartItemResponseFromDetail(detail),
	})
}

// SetCartItemSelected godoc
//
//	@Summary		Set cart item selected
//	@Description	Selects or unselects a cart item for checkout.
//	@Tags			cart
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"Cart item UUID"
//	@Param			request	body		SetSelectedRequest	true	"Selection payload"
//	@Success		200		{object}	CartItemMutationResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		403		{object}	apierror.APIError
//	@Failure		404		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/cart/items/{id}/selected [patch]
func (h *V1Handler) SetCartItemSelected(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	buyerOrgID, err := buyerOrgIDFromCtx(ctx)
	if err != nil {
		return err
	}
	cartItemID, err := parseIDParam(r, "invalid cart item id")
	if err != nil {
		return err
	}

	var req SetSelectedRequest
	if err = decodeJSON(w, r, &req); err != nil {
		return err
	}
	if err = h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	result, err := h.store.SetCartItemSelectedTx(ctx, db.SetCartItemSelectedTxParams{
		CartItemID: cartItemID,
		BuyerOrgID: buyerOrgID,
		IsSelected: *req.IsSelected,
	})
	if err != nil {
		return mapCartWriteError(err)
	}

	detail, err := h.store.GetCartItemDetailsForBuyerOrg(ctx, db.GetCartItemDetailsForBuyerOrgParams{
		CartItemID: result.Item.ID,
		BuyerOrgID: buyerOrgID,
	})
	if err != nil {
		return mapCartWriteError(err)
	}

	return WriteJSON(w, http.StatusOK, CartItemMutationResponse{Item: cartItemResponseFromDetail(detail)})
}

// RemoveCartItem godoc
//
//	@Summary		Remove cart item
//	@Description	Removes a cart item from the authenticated buyer organization's cart.
//	@Tags			cart
//	@Param			id	path	string	true	"Cart item UUID"
//	@Success		204	"No content"
//	@Failure		400	{object}	apierror.APIError
//	@Failure		401	{object}	apierror.APIError
//	@Failure		403	{object}	apierror.APIError
//	@Failure		404	{object}	apierror.APIError
//	@Failure		500	{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/cart/items/{id} [delete]
func (h *V1Handler) RemoveCartItem(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	buyerOrgID, err := buyerOrgIDFromCtx(ctx)
	if err != nil {
		return err
	}
	cartItemID, err := parseIDParam(r, "invalid cart item id")
	if err != nil {
		return err
	}

	if err = h.store.RemoveCartItemTx(ctx, db.RemoveCartItemTxParams{
		BuyerOrgID: buyerOrgID,
		CartItemID: cartItemID,
	}); err != nil {
		return mapCartWriteError(err)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// SetCartShopGroupSelected godoc
//
//	@Summary		Set cart shop selected
//	@Description	Selects or unselects all items in a merchant shop group.
//	@Tags			cart
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"Cart shop group UUID"
//	@Param			request	body		SetSelectedRequest	true	"Selection payload"
//	@Success		200		{object}	CartShopGroupMutationResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		403		{object}	apierror.APIError
//	@Failure		404		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/cart/shop-groups/{id}/selected [patch]
func (h *V1Handler) SetCartShopGroupSelected(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	buyerOrgID, err := buyerOrgIDFromCtx(ctx)
	if err != nil {
		return err
	}
	shopGroupID, err := parseIDParam(r, "invalid cart shop group id")
	if err != nil {
		return err
	}

	var req SetSelectedRequest
	if err = decodeJSON(w, r, &req); err != nil {
		return err
	}
	if err = h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	_, err = h.store.SetCartShopGroupSelectedTx(ctx, db.SetCartShopGroupSelectedTxParams{
		BuyerOrgID:      buyerOrgID,
		CartShopGroupID: shopGroupID,
		IsSelected:      *req.IsSelected,
	})
	if err != nil {
		return mapCartWriteError(err)
	}

	cart, err := h.store.GetCartByBuyerOrgID(ctx, buyerOrgID)
	if err != nil {
		return mapCartWriteError(err)
	}
	details, err := h.store.GetCartDetails(ctx, buyerOrgID)
	if err != nil {
		return fmt.Errorf("failed to get cart details: %w", err)
	}
	cartResp := h.buildCartResponse(ctx, cart, details)
	for _, group := range cartResp.Groups {
		if group.ID == shopGroupID {
			return WriteJSON(w, http.StatusOK, CartShopGroupMutationResponse{ShopGroup: group})
		}
	}

	return apierror.NewAPIError(http.StatusNotFound, errors.New("cart resource not found"))
}

func buyerOrgIDFromCtx(ctx context.Context) (uuid.UUID, error) {
	identity, err := middleware.GetIdentityFromContext(ctx)
	if err != nil {
		return uuid.Nil, apierror.NewAPIError(http.StatusUnauthorized, err)
	}
	if identity.Service != util.SessionServiceBuyerPlatform || identity.OrganizationID == uuid.Nil {
		return uuid.Nil, apierror.NewAPIError(http.StatusForbidden, errors.New("buyer organization required"))
	}
	return identity.OrganizationID, nil
}

func parseIDParam(r *http.Request, message string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, apierror.NewAPIError(http.StatusBadRequest, errors.New(message))
	}
	return id, nil
}

func mapCartWriteError(err error) error {
	if errors.Is(err, db.ErrNotFound) {
		return apierror.NewAPIError(http.StatusNotFound, errors.New("cart resource not found"))
	}
	if db.ErrorCode(err) == db.CheckViolation {
		return apierror.NewAPIError(http.StatusUnprocessableEntity, errors.New("invalid cart data"))
	}
	if db.ErrorCode(err) == db.ForeignKeyViolation {
		return apierror.NewAPIError(http.StatusUnprocessableEntity, errors.New("invalid cart reference"))
	}
	return fmt.Errorf("cart operation failed: %w", err)
}

func (h *V1Handler) buildCartResponse(ctx context.Context, cart db.Cart, rows []db.GetCartDetailsRow) CartResponse {
	resp := CartResponse{
		ID:               cart.ID,
		BuyerOrgID:       cart.BuyerOrgID,
		Subtotal:         decimal.Zero,
		SelectedSubtotal: decimal.Zero,
		TotalQuantity:    0,
		SelectedQuantity: 0,
		Groups:           []CartShopGroupResponse{},
	}
	groupIndex := make(map[uuid.UUID]int, len(rows))
	thumbnailAssets := make([]db.ProductAsset, 0, len(rows))
	for _, row := range rows {
		if row.ThumbnailAssetKey != "" {
			thumbnailAssets = append(thumbnailAssets, db.ProductAsset{
				ID:               0,
				ProductID:        uuid.Nil,
				ProductVariantID: nil,
				AssetKey:         row.ThumbnailAssetKey,
				Type:             "",
				MimeType:         "",
				AltText:          nil,
				SortOrder:        0,
				IsPrimary:        false,
				DurationSeconds:  nil,
			})
		}
	}
	thumbnailURLs := h.resolveAssetURLsParallel(ctx, thumbnailAssets)

	for _, row := range rows {
		idx, ok := groupIndex[row.CartShopGroupID]
		if !ok {
			resp.Groups = append(resp.Groups, CartShopGroupResponse{
				ID:               row.CartShopGroupID,
				MerchantOrgID:    row.MerchantOrgID,
				MerchantName:     row.MerchantName,
				IsSelected:       row.GroupIsSelected,
				Subtotal:         row.Subtotal,
				SelectedSubtotal: decimal.Zero,
				TotalQuantity:    0,
				SelectedQuantity: 0,
				Items:            []CartItemResponse{},
			})
			idx = len(resp.Groups) - 1
			groupIndex[row.CartShopGroupID] = idx
		}

		itemSubtotal := cartItemSubtotal(row.UnitPrice, row.Quantity)
		item := CartItemResponse{
			ID:               row.CartItemID,
			ProductVariantID: row.ProductVariantID,
			ProductID:        row.ProductID,
			ProductName:      row.ProductName,
			ProductSlug:      row.ProductSlug,
			VariantName:      row.VariantName,
			Sku:              row.Sku,
			Quantity:         row.Quantity,
			UnitPrice:        row.UnitPrice,
			Subtotal:         itemSubtotal,
			CurrentPrice:     row.CurrentPrice,
			IsSelected:       row.ItemIsSelected,
			VariantIsActive:  row.VariantIsActive,
			ProductStatus:    row.ProductStatus,
			ThumbnailURL:     nil,
			ThumbnailSource:  nil,
		}
		if row.ThumbnailAssetKey != "" {
			if url := thumbnailURLs[row.ThumbnailAssetKey]; url != "" {
				item.ThumbnailURL = &url
			}
			if row.ThumbnailSource != "" {
				item.ThumbnailSource = &row.ThumbnailSource
			}
		}

		resp.Groups[idx].TotalQuantity += int32(row.Quantity)
		resp.TotalQuantity += int32(row.Quantity)
		resp.Subtotal = resp.Subtotal.Add(itemSubtotal)
		if row.ItemIsSelected {
			resp.Groups[idx].SelectedQuantity += int32(row.Quantity)
			resp.Groups[idx].SelectedSubtotal = resp.Groups[idx].SelectedSubtotal.Add(itemSubtotal)
			resp.SelectedQuantity += int32(row.Quantity)
			resp.SelectedSubtotal = resp.SelectedSubtotal.Add(itemSubtotal)
		}

		resp.Groups[idx].Items = append(resp.Groups[idx].Items, item)
	}

	return resp
}

func cartItemResponseFromTx(item db.CartItem, variant db.GetActiveVariantForCartRow) CartItemResponse {
	return CartItemResponse{
		ID:               item.ID,
		ProductVariantID: item.ProductVariantID,
		ProductID:        variant.ProductID,
		ProductName:      variant.ProductName,
		ProductSlug:      variant.ProductSlug,
		VariantName:      variant.VariantName,
		Sku:              variant.Sku,
		Quantity:         item.Quantity,
		UnitPrice:        item.UnitPrice,
		Subtotal:         cartItemSubtotal(item.UnitPrice, item.Quantity),
		CurrentPrice:     variant.Price,
		IsSelected:       item.IsSelected,
		VariantIsActive:  variant.VariantIsActive,
		ProductStatus:    variant.ProductStatus,
		ThumbnailURL:     nil,
		ThumbnailSource:  nil,
	}
}

func cartItemResponseFromDetail(item db.GetCartItemDetailsForBuyerOrgRow) CartItemResponse {
	return CartItemResponse{
		ID:               item.CartItemID,
		ProductVariantID: item.ProductVariantID,
		ProductID:        item.ProductID,
		ProductName:      item.ProductName,
		ProductSlug:      item.ProductSlug,
		VariantName:      item.VariantName,
		Sku:              item.Sku,
		Quantity:         item.Quantity,
		UnitPrice:        item.UnitPrice,
		Subtotal:         cartItemSubtotal(item.UnitPrice, item.Quantity),
		CurrentPrice:     item.CurrentPrice,
		IsSelected:       item.ItemIsSelected,
		VariantIsActive:  item.VariantIsActive,
		ProductStatus:    item.ProductStatus,
		ThumbnailURL:     nil,
		ThumbnailSource:  nil,
	}
}

func cartItemSubtotal(unitPrice decimal.Decimal, quantity int16) decimal.Decimal {
	return unitPrice.Mul(decimal.NewFromInt(int64(quantity)))
}
