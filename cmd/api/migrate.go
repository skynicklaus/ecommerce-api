package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
)

//go:embed migrations
var migrationsFS embed.FS

const migrateDirectionArgIdx = 2

func runMigrate(ctx context.Context, direction string, log *slog.Logger) error {
	dbSource := os.Getenv("DB_SOURCE")
	if dbSource == "" {
		return errors.New("DB_SOURCE env var not set")
	}

	pool, err := pgxpool.New(ctx, dbSource)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer pool.Close()

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()

	if err = configureGoose(ctx, sqlDB); err != nil {
		return err
	}

	switch direction {
	case "up":
		if err = goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
			return fmt.Errorf("migration up failed: %w", err)
		}
		log.InfoContext(ctx, "migrations applied successfully")
		return bootstrapPlatformAdmin(ctx, db.NewStore(pool), log)

	case "down":
		if err = goose.DownToContext(ctx, sqlDB, "migrations", 0); err != nil {
			return fmt.Errorf("migration down failed: %w", err)
		}
		log.InfoContext(ctx, "migrations rolled back successfully")
		return nil

	default:
		return fmt.Errorf("unknown direction %q — use 'up' or 'down'", direction)
	}
}

func configureGoose(ctx context.Context, sqlDB *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	// Verify the connection is live before running migrations.
	return sqlDB.PingContext(ctx)
}
