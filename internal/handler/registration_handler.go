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
	Name     string `json:"name"     validate:"required,max=255"`
	Email    string `json:"email"    validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,min=8,max=72"`
	RoleSlug string `json:"roleSlug" validate:"required"`
}

type UseerCredentialRegistrationResults struct {
	StatusCode int               `json:"statusCode"`
	User       db.RegisteredUser `json:"user"`
}

func (h *V1Handler) UserCredentialRegistration(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	req := new(UserCredentialRegistrationRequest)
	if err := decodeJSON(w, r, req); err != nil {
		return err
	}

	if err := h.validate(req); err != nil {
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

func (h *V1Handler) PlatformUserCredentialRegistration(
	w http.ResponseWriter,
	r *http.Request,
) error {
	ctx := r.Context()

	req := new(UserCredentialRegistrationRequest)
	if err := decodeJSON(w, r, req); err != nil {
		return err
	}

	if err := h.validate(req); err != nil {
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

	resp := UseerCredentialRegistrationResults{
		StatusCode: http.StatusCreated,
		User:       txResult.User,
	}

	return WriteJSON(w, resp.StatusCode, resp)
}

func (h *V1Handler) CustomerCredentialRegistration(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	req := new(UserCredentialRegistrationRequest)
	if err := decodeJSON(w, r, req); err != nil {
		return err
	}

	if err := h.validate(req); err != nil {
		return apierror.ErrValidation(err)
	}

	hashedPassword, err := password.HashPassword(req.Password)
	if err != nil {
		return err
	}

	txResult, err := registerNewCustomer(ctx, h.store, h.cache, req, hashedPassword)
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
	cache *cache.RedisClient,
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
			ParentID: nil,
			Name:     req.Name,
			Slug:     slugifyEmail(req.Email),
			Status:   string(util.OrganizationStatusActive),
			Type:     string(util.OrganizationTypeIndividual),
			Metadata: []byte("{}"),
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
