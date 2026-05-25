//go:build integration

package db_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/goleak"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
)

var testStore db.Store
var testPool *pgxpool.Pool

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

	testPool = connPool
	testStore = db.NewStore(connPool)

	code := m.Run()

	// Clean up connection pool before goleak check
	connPool.Close()

	if code == 0 {
		if err := goleak.Find(); err != nil {
			logger.Error("goleak: found unexpected goroutines", slog.Any("err", err))
			os.Exit(1)
		}
	}
	os.Exit(code)
}
