package main

//	@title						Ecommerce API
//	@version					1.0
//	@description				Merchant and storefront ecommerce API.
//	@host						localhost:8080
//	@BasePath					/v1
//	@schemes					http https
//	@securityDefinitions.apikey	Bearer
//	@in							header
//	@name						Authorization
//	@description				Type "Bearer" followed by a space and the session token.

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

	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		direction := "up"
		if len(os.Args) > migrateDirectionArgIdx {
			direction = os.Args[migrateDirectionArgIdx]
		}
		if err := runMigrate(context.Background(), direction, logger.Logger); err != nil {
			logger.Error("migration failed", slog.Any("err", err))
			os.Exit(1)
		}
		return
	}

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

	config, err := pgxpool.ParseConfig(dbSource)
	if err != nil {
		return nil, err
	}
	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = time.Minute

	connPool, err := pgxpool.NewWithConfig(context.Background(), config)
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
