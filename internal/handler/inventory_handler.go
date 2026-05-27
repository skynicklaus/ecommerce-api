package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
)

type UpsertInventoryRequest struct {
	ProductVariantID  uuid.UUID `json:"productVariantId"  validate:"required"`
	WarehouseID       int64     `json:"warehouseId"       validate:"required,gt=0"`
	QuantityOnHand    int32     `json:"quantityOnHand"    validate:"gte=0"`
	LowStockThreshold *int32    `json:"lowStockThreshold" validate:"omitempty,gte=0"`
	IsActive          *bool     `json:"isActive"          validate:"required"`
}

type InventoryResponse struct {
	ProductVariantID  uuid.UUID `json:"productVariantId"`
	WarehouseID       int64     `json:"warehouseId"`
	QuantityOnHand    int32     `json:"quantityOnHand"`
	QuantityReserved  int32     `json:"quantityReserved"`
	QuantityAvailable int32     `json:"quantityAvailable"`
	LowStockThreshold *int32    `json:"lowStockThreshold,omitempty"`
	IsActive          bool      `json:"isActive"`
	ProductID         uuid.UUID `json:"productId,omitempty"`
	ProductName       string    `json:"productName,omitempty"`
	VariantSku        string    `json:"variantSku,omitempty"`
	VariantName       string    `json:"variantName,omitempty"`
	WarehouseName     string    `json:"warehouseName,omitempty"`
}

// UpsertInventory godoc
//
//	@Summary		Upsert inventory
//	@Description	Creates or updates inventory for a product variant in a warehouse.
//	@Tags			inventory
//	@Accept			json
//	@Produce		json
//	@Param			request	body		UpsertInventoryRequest	true	"Inventory payload"
//	@Success		200		{object}	InventoryResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		403		{object}	apierror.APIError
//	@Failure		404		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/merchant/inventory [put]
func (h *V1Handler) UpsertInventory(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	organization, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	var req UpsertInventoryRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	inv, err := h.store.UpsertInventory(ctx, db.UpsertInventoryParams{
		OrganizationID:    organization.ID,
		ProductVariantID:  req.ProductVariantID,
		WarehouseID:       req.WarehouseID,
		QuantityOnHand:    req.QuantityOnHand,
		LowStockThreshold: req.LowStockThreshold,
		IsActive:          *req.IsActive,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return apierror.NewAPIError(
				http.StatusNotFound,
				errors.New("product variant or warehouse not found"),
			)
		}
		return mapInventoryWriteError(err)
	}

	row, err := h.store.GetInventoryByVariantAndWarehouseForOrganization(
		ctx,
		db.GetInventoryByVariantAndWarehouseForOrganizationParams{
			OrganizationID:   organization.ID,
			ProductVariantID: inv.ProductVariantID,
			WarehouseID:      inv.WarehouseID,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to reload inventory: %w", err)
	}

	return WriteJSON(w, http.StatusOK, inventoryDetailResponse(row))
}

type ListInventoryResponse struct {
	Data       []InventoryResponse `json:"data"`
	NextCursor *string             `json:"nextCursor,omitempty"`
}

type inventoryCursorPayload struct {
	ProductName      string    `json:"productName"`
	VariantName      string    `json:"variantName"`
	WarehouseName    string    `json:"warehouseName"`
	ProductVariantID uuid.UUID `json:"productVariantId"`
	WarehouseID      int64     `json:"warehouseId"`
}

// ListInventory godoc
//
//	@Summary		List inventory
//	@Description	Lists merchant inventory by organization, or filters by variantId when provided.
//	@Tags			inventory
//	@Produce		json
//	@Param			variantId	query		string	false	"Product variant UUID"
//	@Param			limit		query		int		false	"Page size for organization inventory"	minimum(1)	maximum(100)	default(50)
//	@Param			cursor		query		string	false	"Opaque cursor returned from a previous organization inventory page"
//	@Success		200			{object}	ListInventoryResponse
//	@Failure		400			{object}	apierror.APIError
//	@Failure		401			{object}	apierror.APIError
//	@Failure		403			{object}	apierror.APIError
//	@Failure		500			{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/merchant/inventory [get]
func (h *V1Handler) ListInventory(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	organization, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	var (
		rows []db.ListInventoryByOrganizationRow
		err  error
	)

	variantIDRaw := r.URL.Query().Get("variantId")
	if variantIDRaw != "" {
		variantID, parseErr := uuid.Parse(variantIDRaw)
		if parseErr != nil {
			return apierror.NewAPIError(http.StatusBadRequest, errors.New("invalid variant id"))
		}
		var variantRows []db.ListInventoryByVariantRow
		variantRows, err = h.store.ListInventoryByVariant(ctx, db.ListInventoryByVariantParams{
			OrganizationID:   organization.ID,
			ProductVariantID: variantID,
		})
		if err != nil {
			return fmt.Errorf("failed to list variant inventory: %w", err)
		}

		resp := make([]InventoryResponse, len(variantRows))
		for i, row := range variantRows {
			resp[i] = inventoryVariantRowResponse(row)
		}

		return WriteJSON(w, http.StatusOK, ListInventoryResponse{Data: resp, NextCursor: nil})
	}

	limit := parseLimit(r)
	queryLimit := limit + 1
	cursor, hasCursor, cursorErr := decodeInventoryCursor(r.URL.Query().Get("cursor"))
	if cursorErr != nil {
		return apierror.NewAPIError(
			http.StatusBadRequest,
			fmt.Errorf("invalid cursor: %w", cursorErr),
		)
	}

	rows, err = h.store.ListInventoryByOrganization(ctx, db.ListInventoryByOrganizationParams{
		OrganizationID:        organization.ID,
		HasCursor:             hasCursor,
		AfterProductName:      cursor.ProductName,
		AfterVariantName:      cursor.VariantName,
		AfterWarehouseName:    cursor.WarehouseName,
		AfterProductVariantID: cursor.ProductVariantID,
		AfterWarehouseID:      cursor.WarehouseID,
		PageLimit:             queryLimit,
	})
	if err != nil {
		return fmt.Errorf("failed to list inventory: %w", err)
	}

	var nextCursor *string
	if len(rows) > int(limit) {
		last := rows[limit-1]
		encoded := encodeInventoryCursor(inventoryCursorPayload{
			ProductName:      last.ProductName,
			VariantName:      last.ProductVariantName,
			WarehouseName:    last.WarehouseName,
			ProductVariantID: last.ProductVariantID,
			WarehouseID:      last.WarehouseID,
		})
		nextCursor = &encoded
		rows = rows[:limit]
	}

	resp := make([]InventoryResponse, len(rows))
	for i, row := range rows {
		resp[i] = inventoryOrganizationRowResponse(row)
	}

	return WriteJSON(w, http.StatusOK, ListInventoryResponse{
		Data:       resp,
		NextCursor: nextCursor,
	})
}

// ListProductInventory godoc
//
//	@Summary		List product inventory
//	@Description	Lists inventory rows for all variants of a merchant product.
//	@Tags			inventory
//	@Produce		json
//	@Param			id	path		string	true	"Product UUID"
//	@Success		200	{object}	ListInventoryResponse
//	@Failure		400	{object}	apierror.APIError
//	@Failure		401	{object}	apierror.APIError
//	@Failure		403	{object}	apierror.APIError
//	@Failure		500	{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/merchant/products/{id}/inventory [get]
func (h *V1Handler) ListProductInventory(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	organization, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	productID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("invalid product id"))
	}

	rows, err := h.store.ListInventoryByProduct(ctx, db.ListInventoryByProductParams{
		OrganizationID: organization.ID,
		ProductID:      productID,
	})
	if err != nil {
		return fmt.Errorf("failed to list product inventory: %w", err)
	}

	resp := make([]InventoryResponse, len(rows))
	for i, row := range rows {
		resp[i] = inventoryProductRowResponse(row)
	}

	return WriteJSON(w, http.StatusOK, ListInventoryResponse{Data: resp, NextCursor: nil})
}

func inventoryDetailResponse(
	row db.GetInventoryByVariantAndWarehouseForOrganizationRow,
) InventoryResponse {
	return InventoryResponse{
		ProductVariantID:  row.ProductVariantID,
		WarehouseID:       row.WarehouseID,
		QuantityOnHand:    row.QuantityOnHand,
		QuantityReserved:  row.QuantityReserved,
		QuantityAvailable: int32Value(row.QuantityAvailable),
		LowStockThreshold: row.LowStockThreshold,
		IsActive:          row.IsActive,
		ProductID:         row.ProductID,
		ProductName:       row.ProductName,
		VariantSku:        row.ProductVariantSku,
		VariantName:       row.ProductVariantName,
		WarehouseName:     row.WarehouseName,
	}
}

func inventoryOrganizationRowResponse(row db.ListInventoryByOrganizationRow) InventoryResponse {
	return InventoryResponse{
		ProductVariantID:  row.ProductVariantID,
		WarehouseID:       row.WarehouseID,
		QuantityOnHand:    row.QuantityOnHand,
		QuantityReserved:  row.QuantityReserved,
		QuantityAvailable: int32Value(row.QuantityAvailable),
		LowStockThreshold: row.LowStockThreshold,
		IsActive:          row.IsActive,
		ProductID:         row.ProductID,
		ProductName:       row.ProductName,
		VariantSku:        row.ProductVariantSku,
		VariantName:       row.ProductVariantName,
		WarehouseName:     row.WarehouseName,
	}
}

func inventoryProductRowResponse(row db.ListInventoryByProductRow) InventoryResponse {
	return InventoryResponse{
		ProductVariantID:  row.ProductVariantID,
		WarehouseID:       row.WarehouseID,
		QuantityOnHand:    row.QuantityOnHand,
		QuantityReserved:  row.QuantityReserved,
		QuantityAvailable: int32Value(row.QuantityAvailable),
		LowStockThreshold: row.LowStockThreshold,
		IsActive:          row.IsActive,
		ProductID:         row.ProductID,
		ProductName:       row.ProductName,
		VariantSku:        row.ProductVariantSku,
		VariantName:       row.ProductVariantName,
		WarehouseName:     row.WarehouseName,
	}
}

func inventoryVariantRowResponse(row db.ListInventoryByVariantRow) InventoryResponse {
	return InventoryResponse{
		ProductVariantID:  row.ProductVariantID,
		WarehouseID:       row.WarehouseID,
		QuantityOnHand:    row.QuantityOnHand,
		QuantityReserved:  row.QuantityReserved,
		QuantityAvailable: int32Value(row.QuantityAvailable),
		LowStockThreshold: row.LowStockThreshold,
		IsActive:          row.IsActive,
		ProductID:         row.ProductID,
		ProductName:       row.ProductName,
		VariantSku:        row.ProductVariantSku,
		VariantName:       row.ProductVariantName,
		WarehouseName:     row.WarehouseName,
	}
}

func encodeInventoryCursor(cursor inventoryCursorPayload) string {
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeInventoryCursor(rawCursor string) (inventoryCursorPayload, bool, error) {
	if rawCursor == "" {
		return inventoryCursorPayload{}, false, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(rawCursor)
	if err != nil {
		return inventoryCursorPayload{}, false, errors.New("malformed cursor encoding")
	}

	var cursor inventoryCursorPayload
	if unmarshallErr := json.Unmarshal(raw, &cursor); unmarshallErr != nil {
		return inventoryCursorPayload{}, false, errors.New("malformed cursor payload")
	}
	if cursor.ProductName == "" ||
		cursor.VariantName == "" ||
		cursor.WarehouseName == "" ||
		cursor.ProductVariantID == uuid.Nil ||
		cursor.WarehouseID <= 0 {
		return inventoryCursorPayload{}, false, errors.New("malformed cursor values")
	}

	return cursor, true, nil
}

func int32Value(v *int32) int32 {
	// quantity_available is generated from non-null quantities; nil is only a defensive fallback.
	if v == nil {
		return 0
	}
	return *v
}

func mapInventoryWriteError(err error) error {
	switch db.ErrorCode(err) {
	case db.CheckViolation:
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("invalid inventory data"))
	case db.ForeignKeyViolation:
		return apierror.NewAPIError(
			http.StatusBadRequest,
			errors.New("invalid inventory reference"),
		)
	default:
		return err
	}
}
