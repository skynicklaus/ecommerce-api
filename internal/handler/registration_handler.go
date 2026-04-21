package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
	"github.com/skynicklaus/ecommerce-api/internal/password"
	"github.com/skynicklaus/ecommerce-api/util"
)

const (
	RoleIndividualOwner = "individual.owner"
)

type UserCredentialRegistrationRequest struct {
	Name     string `json:"name"     validate:"required"`
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	RoleSlug string `json:"roleSlug" validate:"required"`
}

type UseerCredentialRegistrationResults struct {
	StatusCode int               `json:"statusCode"`
	User       db.RegisteredUser `json:"user"`
}

func (h *V1Handler) UserCredentialRegistration(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	req := new(UserCredentialRegistrationRequest)
	if err := decodeJSON(r, req); err != nil {
		return errInvalidJSON()
	}

	if err := h.validate(req); err != nil {
		return errValidation(err)
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
			return NewAPIError(http.StatusConflict, "email is taken", nil)
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
	if err := decodeJSON(r, req); err != nil {
		return errInvalidJSON()
	}

	if err := h.validate(req); err != nil {
		return errValidation(err)
	}

	role, err := h.cache.GetSystemPlatformRoleFromSlug(ctx, req.RoleSlug)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return NewAPIError(http.StatusBadRequest, "invalid role", nil)
		}

		return err
	}

	if role.OrganizationType != string(util.OrganizationTypePlatform) {
		return NewAPIError(http.StatusBadRequest, "invalid role organization type", nil)
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
	if err := decodeJSON(r, req); err != nil {
		return errInvalidJSON()
	}

	if err := h.validate(req); err != nil {
		return errValidation(err)
	}

	customer, err := h.store.GetCustomerByEmail(ctx, req.Email)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return err
	}

	hashedPassword, hashErr := password.HashPassword(req.Password)
	if hashErr != nil {
		return hashErr
	}

	var resultID uuid.UUID
	if errors.Is(err, db.ErrNotFound) {
		txResult, registerErr := registerNewCustomer(
			ctx,
			h.store,
			h.cache,
			req,
			hashedPassword,
		)
		if registerErr != nil {
			return registerErr
		}

		resultID = txResult.User.ID
	} else {
		accountInfo, accountErr := createCredentialAccount(
			ctx,
			h.store,
			util.IdentityCustomer,
			customer.ID,
			hashedPassword,
		)
		if accountErr != nil {
			return accountErr
		}

		resultID = accountInfo.ID
	}

	return WriteJSON(w, http.StatusCreated, map[string]string{
		"id": resultID.String(),
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
			Metadata: []byte{},
		},
	}

	txResults, err := store.CustomerRegistrationTx(ctx, arg)
	if err != nil {
		errCode := db.ErrorCode(err)
		if errCode == db.UniqueViolation {
			return db.CustomerRegistrationTxResults{}, NewAPIError(
				http.StatusConflict,
				"email is taken",
				nil,
			)
		}

		return db.CustomerRegistrationTxResults{}, err
	}

	return txResults, nil
}

func createCredentialAccount(
	ctx context.Context,
	store db.Store,
	identityType util.IdentityType,
	uuid uuid.UUID,
	hashedPassword string,
) (db.AccountInfo, error) {
	var accountInfo db.AccountInfo

	switch identityType {
	case util.IdentityUser:
		userAccount, err := store.CreateUserAccount(ctx, db.CreateUserAccountParams{
			UserID:                uuid,
			ProviderID:            string(util.ProviderIDCredential),
			AccountID:             uuid.String(),
			HashedPassword:        &hashedPassword,
			AccessToken:           nil,
			RefreshToken:          nil,
			AccessTokenExpiresAt:  nil,
			RefreshTokenExpiresAt: nil,
			IDToken:               nil,
			Scope:                 nil,
		})
		if err != nil {
			errCode := db.ErrorCode(err)
			if errCode == db.UniqueViolation {
				return db.AccountInfo{}, NewAPIError(
					http.StatusConflict,
					"account already registered",
					nil,
				)
			}

			return db.AccountInfo{}, err
		}

		accountInfo = db.MapUserAccountToAccountInfo(userAccount)
	case util.IdentityCustomer:
		customerAccount, err := store.CreateCustomerAccount(ctx, db.CreateCustomerAccountParams{
			CustomerID:            uuid,
			ProviderID:            string(util.ProviderIDCredential),
			AccountID:             uuid.String(),
			HashedPassword:        &hashedPassword,
			AccessToken:           nil,
			RefreshToken:          nil,
			AccessTokenExpiresAt:  nil,
			RefreshTokenExpiresAt: nil,
			IDToken:               nil,
			Scope:                 nil,
		})
		if err != nil {
			errCode := db.ErrorCode(err)
			if errCode == db.UniqueViolation {
				return db.AccountInfo{}, NewAPIError(
					http.StatusConflict,
					"account already registered",
					nil,
				)
			}

			return db.AccountInfo{}, err
		}

		accountInfo = db.MapCustomerAccountToAccountInfo(customerAccount)
	default:
		return db.AccountInfo{}, errors.New("invalid identity type")
	}
	return accountInfo, nil
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
