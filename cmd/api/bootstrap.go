package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/password"
	"github.com/skynicklaus/ecommerce-api/util"
)

// bootstrapPlatformAdmin creates the first platform administrator from environment
// variables. It is a no-op when a platform admin already exists, so it is safe to
// run on every migration. Env vars are only required on the very first run (count == 0);
// subsequent deploys can omit them.
//
// Required env vars (first run only): PLATFORM_ADMIN_EMAIL, PLATFORM_ADMIN_PASSWORD
// Optional env vars: PLATFORM_ADMIN_NAME (default: "Platform Owner")
//
//	PLATFORM_ADMIN_ROLE_SLUG (default: "platform.owner")
func bootstrapPlatformAdmin(ctx context.Context, store db.Store, log *slog.Logger) error {
	count, err := store.CountPlatformAdmins(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap check failed: %w", err)
	}
	if count > 0 {
		log.InfoContext(ctx, "platform admin already exists, skipping bootstrap")
		return nil
	}

	adminEmail := os.Getenv("PLATFORM_ADMIN_EMAIL")
	adminPassword := os.Getenv("PLATFORM_ADMIN_PASSWORD")
	if adminEmail == "" || adminPassword == "" {
		return errors.New(
			"no platform admin exists and PLATFORM_ADMIN_EMAIL / PLATFORM_ADMIN_PASSWORD are not set",
		)
	}

	hashedPw, err := password.HashPassword(adminPassword)
	if err != nil {
		return fmt.Errorf("failed to hash bootstrap password: %w", err)
	}

	roleSlug := os.Getenv("PLATFORM_ADMIN_ROLE_SLUG")
	if roleSlug == "" {
		roleSlug = "platform.owner"
	}

	role, err := store.GetRoleBySlug(ctx, roleSlug)
	if err != nil {
		return fmt.Errorf("platform role %q not found — ensure seeds have run: %w", roleSlug, err)
	}

	platformOrg, err := store.GetOrganizationBySlug(ctx, string(util.OrganizationTypePlatform))
	if err != nil {
		return fmt.Errorf("platform organization not found — ensure seeds have run: %w", err)
	}

	adminName := os.Getenv("PLATFORM_ADMIN_NAME")
	if adminName == "" {
		adminName = "Platform Owner"
	}

	_, err = store.PlatformUserRegistrationTx(ctx, db.PlatformUserRegistrationTxParams{
		RoleID:               role.ID,
		RoleOrganizationType: string(util.OrganizationTypePlatform),
		RoleAssignBy:         nil,
		OrganizationID:       platformOrg.ID,
		UserInfo: db.UserInfo{
			Name:  adminName,
			Email: adminEmail,
		},
		AccountInfoParams: db.AccountInfoParams{
			ProviderID:            string(util.ProviderIDCredential),
			HashedPassword:        &hashedPw,
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
		return fmt.Errorf("failed to create platform admin: %w", err)
	}

	log.InfoContext(ctx, "platform admin bootstrapped successfully", "email", adminEmail, "name", adminName)
	return nil
}
