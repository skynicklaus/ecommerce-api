package db

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type RLSContext struct {
	IdentityID      uuid.UUID
	OrganiationID   uuid.UUID
	IsPlatformUser  bool
	IsPlatformAdmin bool
}

type rlsContextKey struct{}

func WithRLSContext(ctx context.Context, rls RLSContext) context.Context {
	return context.WithValue(ctx, rlsContextKey{}, rls)
}

func rlsFromContext(ctx context.Context) (RLSContext, bool) {
	rls, ok := ctx.Value(rlsContextKey{}).(RLSContext)
	return rls, ok
}

func applyRLS(ctx context.Context, tx pgx.Tx) error {
	rls, ok := rlsFromContext(ctx)
	if !ok {
		return nil
	}

	_, err := tx.Exec(
		ctx,
		`
		SET LOCAL ROLE app_user;
		SELECT
			set_config('app.current_identity_id', $1, true),
			set_config('app.current_org_id', $2, true),
			set_config('app.is_platform_user', $3 , true),
			set_config('app.is_platform_admin', $4, true)
		`,
		rls.IdentityID.String(),
		rls.OrganiationID.String(),
		strconv.FormatBool(rls.IsPlatformUser),
		strconv.FormatBool(rls.IsPlatformAdmin),
	)

	return err
}
