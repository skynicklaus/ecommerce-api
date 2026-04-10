package db_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
)

var testStore db.Store

func TestMain(m *testing.M) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	dbSource := os.Getenv("DB_SOURCE")
	if dbSource == "" {
		logger.Error("db source is empty")
		os.Exit(1)
	}

	connPool, err := pgxpool.New(context.Background(), dbSource)
	if err != nil {
		logger.Error("unable to open database connection", slog.Any("err", err))
		os.Exit(1)
	}

	testStore = db.NewStore(connPool)

	os.Exit(m.Run())
}
