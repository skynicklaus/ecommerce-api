package handler

import (
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

type ListInventoryResponse struct {
	Data []InventoryResponse `json:"data"`
}

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

		return WriteJSON(w, http.StatusOK, ListInventoryResponse{Data: resp})
	}

	rows, err = h.store.ListInventoryByOrganization(ctx, organization.ID)
	if err != nil {
		return fmt.Errorf("failed to list inventory: %w", err)
	}

	resp := make([]InventoryResponse, len(rows))
	for i, row := range rows {
		resp[i] = inventoryOrganizationRowResponse(row)
	}

	return WriteJSON(w, http.StatusOK, ListInventoryResponse{Data: resp})
}

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

	return WriteJSON(w, http.StatusOK, ListInventoryResponse{Data: resp})
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
