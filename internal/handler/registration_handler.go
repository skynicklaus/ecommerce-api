package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
	"github.com/skynicklaus/ecommerce-api/internal/password"
	"github.com/skynicklaus/ecommerce-api/util"
)

const (
	RoleIndividualOwner = "individual.owner"
)

type UserCredentialRegistrationRequest struct {
	Name     string `json:"name"     validate:"required,max=255" example:"Jane Merchant"`
	Email    string `json:"email"    validate:"required,email,max=254" example:"jane@example.com"`
	Password string `json:"password" validate:"required,min=8,max=72" example:"correct-horse-battery-staple"`
	RoleSlug string `json:"roleSlug" validate:"required" example:"merchant.owner"`
}

type IDResponse struct {
	ID string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
}

type UserCredentialRegistrationResponse struct {
	StatusCode int               `json:"statusCode"`
	User       db.RegisteredUser `json:"user"`
}

// UserCredentialRegistration godoc
//
//	@Summary		Create merchant user
//	@Description	Creates a user account through the platform-admin surface.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			request	body		UserCredentialRegistrationRequest	true	"User registration payload"
//	@Success		201		{object}	IDResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		403		{object}	apierror.APIError
//	@Failure		409		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/users/merchant [post]
func (h *V1Handler) UserCredentialRegistration(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	var req UserCredentialRegistrationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}

	if err := h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	hashedPassword, err := password.HashPassword(req.Password)
	if err != nil {
		return err
	}

	txResult, err := h.store.UserRegistrationTx(ctx, db.UserRegistrationTxParams{
		UserInfo: db.UserInfo{
			Name:  req.Name,
			Email: req.Email,
		},
		AccountInfoParams: db.AccountInfoParams{
			ProviderID:            string(util.ProviderIDCredential),
			HashedPassword:        &hashedPassword,
			AccountID:             "",
			AccessToken:           nil,
			RefreshToken:          nil,
			AccessTokenExpiresAt:  nil,
			RefreshTokenExpiresAt: nil,
			Scope:                 nil,
			IDToken:               nil,
		},
	})
	if err != nil {
		errCode := db.ErrorCode(err)
		if errCode == db.UniqueViolation {
			return apierror.NewAPIError(http.StatusConflict, errors.New("email is taken"))
		}

		return err
	}

	return WriteJSON(w, http.StatusCreated, map[string]string{
		"id": txResult.User.ID.String(),
	})
}

// PlatformUserCredentialRegistration godoc
//
//	@Summary		Create platform user
//	@Description	Creates another platform-admin user. The first admin is bootstrapped during migration.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			request	body		UserCredentialRegistrationRequest	true	"Platform user registration payload"
//	@Success		201		{object}	UserCredentialRegistrationResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		401		{object}	apierror.APIError
//	@Failure		403		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Security		Bearer
//	@Router			/users/platform [post]
func (h *V1Handler) PlatformUserCredentialRegistration(
	w http.ResponseWriter,
	r *http.Request,
) error {
	ctx := r.Context()

	var req UserCredentialRegistrationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}

	if err := h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	role, err := h.cache.GetSystemPlatformRoleFromSlug(ctx, req.RoleSlug)
	if err != nil {
		if errors.Is(err, cache.ErrRoleNotFound) {
			return apierror.NewAPIError(http.StatusBadRequest, errors.New("invalid role"))
		}

		return err
	}

	if role.OrganizationType != string(util.OrganizationTypePlatform) {
		return apierror.NewAPIError(
			http.StatusBadRequest,
			errors.New("invalid role organization type"),
		)
	}

	hashedPassword, err := password.HashPassword(req.Password)
	if err != nil {
		return err
	}

	platformOrganization, err := h.store.GetOrganizationBySlug(
		ctx,
		string(util.OrganizationTypePlatform),
	)
	if err != nil {
		return err
	}

	arg := db.PlatformUserRegistrationTxParams{
		RoleID:               role.ID,
		RoleOrganizationType: string(util.OrganizationTypePlatform),
		RoleAssignBy:         nil,
		OrganizationID:       platformOrganization.ID,
		UserInfo: db.UserInfo{
			Name:  req.Name,
			Email: req.Email,
		},
		AccountInfoParams: db.AccountInfoParams{
			ProviderID:            string(util.ProviderIDCredential),
			AccountID:             "",
			HashedPassword:        &hashedPassword,
			AccessToken:           nil,
			RefreshToken:          nil,
			AccessTokenExpiresAt:  nil,
			RefreshTokenExpiresAt: nil,
			Scope:                 nil,
			IDToken:               nil,
		},
	}

	txResult, err := h.store.PlatformUserRegistrationTx(ctx, arg)
	if err != nil {
		return err
	}

	resp := UserCredentialRegistrationResponse{
		StatusCode: http.StatusCreated,
		User:       txResult.User,
	}

	return WriteJSON(w, resp.StatusCode, resp)
}

// CustomerCredentialRegistration godoc
//
//	@Summary		Register customer
//	@Description	Creates a customer account and its individual organization.
//	@Tags			customers
//	@Accept			json
//	@Produce		json
//	@Param			request	body		UserCredentialRegistrationRequest	true	"Customer registration payload"
//	@Success		201		{object}	IDResponse
//	@Failure		400		{object}	apierror.APIError
//	@Failure		409		{object}	apierror.APIError
//	@Failure		422		{object}	apierror.APIError
//	@Failure		500		{object}	apierror.APIError
//	@Router			/customer [post]
func (h *V1Handler) CustomerCredentialRegistration(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	var req UserCredentialRegistrationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}

	if err := h.validate(&req); err != nil {
		return apierror.ErrValidation(err)
	}

	hashedPassword, err := password.HashPassword(req.Password)
	if err != nil {
		return err
	}

	txResult, err := registerNewCustomer(ctx, h.store, h.cache, &req, hashedPassword)
	if err != nil {
		return err
	}

	return WriteJSON(w, http.StatusCreated, map[string]string{
		"id": txResult.User.ID.String(),
	})
}

func registerNewCustomer(
	ctx context.Context,
	store db.Store,
	cache *cache.Client,
	req *UserCredentialRegistrationRequest,
	hashedPassword string,
) (db.CustomerRegistrationTxResults, error) {
	role, err := cache.GetSystemIndividualRoleFromSlug(ctx, RoleIndividualOwner)
	if err != nil {
		return db.CustomerRegistrationTxResults{}, err
	}

	arg := db.CustomerRegistrationTxParams{
		RoleOrganizationType: string(util.OrganizationTypeIndividual),
		RoleID:               role.ID,
		RoleAssignBy:         nil,
		UserInfo: db.UserInfo{
			Name:  req.Name,
			Email: req.Email,
		},
		AccountInfoParams: db.AccountInfoParams{
			ProviderID:            string(util.ProviderIDCredential),
			HashedPassword:        &hashedPassword,
			AccountID:             "",
			AccessToken:           nil,
			RefreshToken:          nil,
			AccessTokenExpiresAt:  nil,
			RefreshTokenExpiresAt: nil,
			Scope:                 nil,
			IDToken:               nil,
		},
		CreateOrganizationParams: db.CreateOrganizationParams{
			ParentID:   nil,
			Name:       req.Name,
			Slug:       slugifyEmail(req.Email),
			Status:     string(util.OrganizationStatusActive),
			Type:       string(util.OrganizationTypeIndividual),
			Capability: string(util.OrganizationCapabilityBuyer),
			Metadata:   []byte("{}"),
		},
	}

	txResults, err := store.CustomerRegistrationTx(ctx, arg)
	if err != nil {
		errCode := db.ErrorCode(err)
		if errCode == db.UniqueViolation {
			return db.CustomerRegistrationTxResults{}, apierror.NewAPIError(
				http.StatusConflict,
				errors.New("email is taken"),
			)
		}

		return db.CustomerRegistrationTxResults{}, err
	}

	return txResults, nil
}

func slugifyEmail(email string) string {
	replacer := strings.NewReplacer(
		"@", "-at-",
		".", "-",
		"_", "-",
		"+", "-",
	)
	return strings.ToLower(replacer.Replace(email))
}
