package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/util"
)

type WarehouseAddressRequest struct {
	Label      string  `json:"label"           validate:"required,max=100"`
	Line1      string  `json:"line1"           validate:"required"`
	Line2      *string `json:"line2,omitempty"`
	PostalCode string  `json:"postalCode"      validate:"required"`
	City       string  `json:"city"            validate:"required"`
	State      string  `json:"state"           validate:"required"`
	Country    string  `json:"country"         validate:"required"`
}

type CreateWarehouseRequest struct {
	Name    string                  `json:"name"    validate:"required,max=255"`
	Address WarehouseAddressRequest `json:"address" validate:"required"`
}

type WarehouseAddressResponse struct {
	ID         uuid.UUID `json:"id"`
	Label      string    `json:"label"`
	Line1      string    `json:"line1"`
	Line2      *string   `json:"line2,omitempty"`
	PostalCode string    `json:"postalCode"`
	City       string    `json:"city"`
	State      string    `json:"state"`
	Country    string    `json:"country"`
}

type WarehouseResponse struct {
	ID             int64                    `json:"id"`
	OrganizationID uuid.UUID                `json:"organizationId"`
	Name           string                   `json:"name"`
	IsActive       bool                     `json:"isActive"`
	Address        WarehouseAddressResponse `json:"address"`
}

type ListWarehousesResponse struct {
	Data []WarehouseResponse `json:"data"`
}

// CreateWarehouse godoc
//
//	@Summary		Create warehouse
//	@Description	Creates a merchant warehouse and its warehouse address.
//	@Tags			warehouses
//	@Accept			json
//	@Produce		json
//	@Param			request	body		CreateWarehouseRequest	true	"Warehouse payload"
//	@Success		201		{object}	WarehouseResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		403		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/merchant/warehouses [post]
func (h *V1Handler) CreateWarehouse(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	organization, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	var req CreateWarehouseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	result, err := h.store.CreateWarehouseTx(ctx, db.CreateWarehouseTxParams{
		OrganizationID: organization.ID,
		Name:           req.Name,
		Address:        buildCreateWarehouseAddressParams(req.Address),
	})
	if err != nil {
		return mapWarehouseWriteError(err)
	}

	return WriteJSON(w, http.StatusCreated, warehouseTxResponse(result))
}

// ListWarehouses godoc
//
//	@Summary		List warehouses
//	@Description	Lists warehouses for the active merchant organization.
//	@Tags			warehouses
//	@Produce		json
//	@Success		200	{object}	ListWarehousesResponse
//	@Failure		401	{object}	apierror.APIError
//	@Failure		403	{object}	apierror.APIError
//	@Failure		500	{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/merchant/warehouses [get]
func (h *V1Handler) ListWarehouses(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	organization, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	rows, err := h.store.ListWarehousesByOrganization(ctx, organization.ID)
	if err != nil {
		return fmt.Errorf("failed to list warehouses: %w", err)
	}

	resp := make([]WarehouseResponse, len(rows))
	for i, row := range rows {
		resp[i] = warehouseListRowResponse(row)
	}

	return WriteJSON(w, http.StatusOK, ListWarehousesResponse{Data: resp})
}

type UpdateWarehouseRequest struct {
	Name     string                  `json:"name"     validate:"required,max=255"`
	IsActive *bool                   `json:"isActive" validate:"required"`
	Address  WarehouseAddressRequest `json:"address"  validate:"required"`
}

// UpdateWarehouse godoc
//
//	@Summary		Update warehouse
//	@Description	Updates a merchant warehouse and its warehouse address.
//	@Tags			warehouses
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int						true	"Warehouse ID"
//	@Param			request	body		UpdateWarehouseRequest	true	"Warehouse payload"
//	@Success		200		{object}	WarehouseResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		403		{object}	apierror.APIError
//	@Failure		404		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/merchant/warehouses/{id} [put]
func (h *V1Handler) UpdateWarehouse(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	organization, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	warehouseID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || warehouseID <= 0 {
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("invalid warehouse id"))
	}

	var req UpdateWarehouseRequest
	if err = decodeJSON(w, r, &req); err != nil {
		return err
	}
	if err = h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	result, err := h.store.UpdateWarehouseTx(ctx, db.UpdateWarehouseTxParams{
		ID:             warehouseID,
		OrganizationID: organization.ID,
		Name:           req.Name,
		IsActive:       *req.IsActive,
		Address:        buildUpdateWarehouseAddressParams(req.Address),
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return apierror.NewAPIError(http.StatusNotFound, errors.New("warehouse not found"))
		}
		return mapWarehouseWriteError(err)
	}

	return WriteJSON(w, http.StatusOK, warehouseTxResponse(result))
}

func buildCreateWarehouseAddressParams(
	req WarehouseAddressRequest,
) db.CreateAddressParams {
	//nolint:exhaustruct // set org id in transaction
	return db.CreateAddressParams{
		Type:       string(util.AddressWarehouse),
		Label:      req.Label,
		Line1:      req.Line1,
		Line2:      req.Line2,
		PostalCode: req.PostalCode,
		City:       req.City,
		State:      req.State,
		Country:    req.Country,
	}
}

func buildUpdateWarehouseAddressParams(
	req WarehouseAddressRequest,
) db.UpdateAddressByIDAndOrganizationParams {
	//nolint:exhaustruct // set id and org id in transaction
	return db.UpdateAddressByIDAndOrganizationParams{
		Label:      req.Label,
		Line1:      req.Line1,
		Line2:      req.Line2,
		PostalCode: req.PostalCode,
		City:       req.City,
		State:      req.State,
		Country:    req.Country,
	}
}

func warehouseTxResponse(result db.WarehouseTxResult) WarehouseResponse {
	return WarehouseResponse{
		ID:             result.Warehouse.ID,
		OrganizationID: result.Warehouse.OrganizationID,
		Name:           result.Warehouse.Name,
		IsActive:       result.Warehouse.IsActive,
		Address: WarehouseAddressResponse{
			ID:         result.Address.ID,
			Label:      result.Address.Label,
			Line1:      result.Address.Line1,
			Line2:      result.Address.Line2,
			PostalCode: result.Address.PostalCode,
			City:       result.Address.City,
			State:      result.Address.State,
			Country:    result.Address.Country,
		},
	}
}

func warehouseListRowResponse(row db.ListWarehousesByOrganizationRow) WarehouseResponse {
	return WarehouseResponse{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		Name:           row.Name,
		IsActive:       row.IsActive,
		Address: WarehouseAddressResponse{
			ID:         row.AddressID,
			Label:      row.AddressLabel,
			Line1:      row.AddressLine1,
			Line2:      row.AddressLine2,
			PostalCode: row.AddressPostalCode,
			City:       row.AddressCity,
			State:      row.AddressState,
			Country:    row.AddressCountry,
		},
	}
}

func mapWarehouseWriteError(err error) error {
	switch db.ErrorCode(err) {
	case db.CheckViolation:
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("invalid warehouse data"))
	case db.ForeignKeyViolation:
		return apierror.NewAPIError(
			http.StatusBadRequest,
			errors.New("invalid warehouse reference"),
		)
	default:
		return err
	}
}
