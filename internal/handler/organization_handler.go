package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
	"github.com/skynicklaus/ecommerce-api/util"
)

type CreateOrganizationRequest struct {
	IdentityID uuid.UUID       `json:"identityId" validate:"required"`
	ParentID   *uuid.UUID      `json:"parentId"   validate:"omitempty"`
	Name       string          `json:"name"       validate:"required,max=255"`
	Slug       string          `json:"slug"       validate:"required"`
	Type       string          `json:"type"       validate:"required,oneof=merchant"`
	Status     string          `json:"status"     validate:"required,oneof=pending active suspended"`
	Metadata   json.RawMessage `json:"metadata"   validate:"required"`
	RoleSlug   string          `json:"roleSlug"   validate:"required"`
}

func (h *V1Handler) CreateOrganization(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	req := new(CreateOrganizationRequest)
	if err := decodeJSON(w, r, req); err != nil {
		return err
	}

	if err := h.validate(req); err != nil {
		return apierror.ErrValidation(err)
	}

	role, err := getRoleFromSlug(ctx, h.cache, req.Type, req.RoleSlug)
	if err != nil {
		if errors.Is(err, cache.ErrRoleNotFound) {
			return apierror.NewAPIError(http.StatusBadRequest, errors.New("invalid role"))
		}

		return err
	}

	if req.Type != role.OrganizationType {
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("organization type mismatch"))
	}

	txResult, err := h.store.CreateOrganizationTx(ctx, db.CreateOrganizationTxRequest{
		IdentityID:           req.IdentityID,
		ParentID:             req.ParentID,
		Name:                 req.Name,
		Slug:                 req.Slug,
		Type:                 req.Type,
		Status:               string(util.OrganizationStatusPending),
		Capability:           string(util.OrganizationCapabilitySeller),
		Metadata:             []byte(req.Metadata),
		RoleID:               role.ID,
		RoleOrganizationType: role.OrganizationType,
	})
	if err != nil {
		return err
	}

	return WriteJSON(w, http.StatusCreated, map[string]string{
		"id": txResult.Organization.ID.String(),
	})
}

func getRoleFromSlug(
	ctx context.Context,
	cache *cache.Client,
	organizationType string,
	slug string,
) (db.Role, error) {
	switch organizationType {
	case string(util.OrganizationTypeMerchant):
		return cache.GetSystemMerchantRoleFromSlug(ctx, slug)
	case string(util.OrganizationTypeCompany):
		return cache.GetSystemCompanyRoleFromSlug(ctx, slug)
	default:
		return db.Role{}, errors.New("invalid organization type")
	}
}
