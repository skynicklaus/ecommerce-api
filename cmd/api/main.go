package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/server"
	"github.com/skynicklaus/ecommerce-api/util"
)

const (
	ContextTimeout = 5 * time.Second
)

func main() {
	logger := util.NewLogger()

	slog.SetDefault(logger.Logger)
	slog.SetLogLoggerLevel(slog.LevelError)

	errC, err := run(logger)
	if err != nil {
		logger.Fatal("Couldn't run server", slog.Any("err", err))
	}

	if chanErr := <-errC; chanErr != nil {
		logger.Fatal("Error while running server", slog.Any("err", chanErr))
	}
}

func run(logger *util.ServerLogger) (<-chan error, error) {
	dbSource := os.Getenv("DB_SOURCE")
	if dbSource == "" {
		return nil, errors.New("no DB_SOURCE env var set")
	}

	connPool, err := pgxpool.New(context.Background(), dbSource)
	if err != nil {
		return nil, err
	}

	store := db.NewStore(connPool)

	server := server.NewServer(store, logger)

	sc := make(chan error, 1)
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGINT,
	)

	go func() {
		<-ctx.Done()

		logger.Warn("shutting down gracefully, press Ctrl+C again to force")
		stop()

		logger.Info("received shutdown signal")

		ctxTimeout, cancel := context.WithTimeout(context.Background(), ContextTimeout)

		defer func() {
			connPool.Close()
			cancel()
			close(sc)
		}()

		server.SetKeepAlivesEnabled(false)

		if shutdownErr := server.Shutdown(ctxTimeout); shutdownErr != nil {
			sc <- shutdownErr
		}

		logger.Info("shutdown completed")
	}()

	go func() {
		if serveErr := server.ListenAndServe(); serveErr != nil &&
			serveErr != http.ErrServerClosed {
			sc <- serveErr
		}
	}()

	logger.Info("server started", "port", server.Addr)

	return sc, nil
}
