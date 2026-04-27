package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
	"github.com/skynicklaus/ecommerce-api/internal/storage"
	"github.com/skynicklaus/ecommerce-api/util"
)

const (
	ServerReadTimeout       = 10 * time.Second
	ServerReadHeaderTimouet = 10 * time.Second
	ServerWriteTimeout      = 20 * time.Second
	ServerIdleTimeout       = 120 * time.Second
)

type Server struct {
	port    int
	logger  *util.ServerLogger
	store   db.Store
	redis   *cache.RedisClient
	storage *storage.S3Storage
}

func NewServer(store db.Store, logger *util.ServerLogger) *http.Server {
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	if port == 0 {
		port = 8080
	}

	redis := cache.NewRedis(store, logger)

	storage, err := storage.New(context.Background())
	if err != nil {
		logger.Fatal("failed to initialize s3 storage client", slog.Any("err", err))
	}

	newServer := &Server{
		port:    port,
		logger:  logger,
		store:   store,
		redis:   redis,
		storage: storage,
	}

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", newServer.port),
		Handler:           newServer.RegisterRoutes(),
		ReadTimeout:       ServerReadTimeout,
		ReadHeaderTimeout: ServerReadHeaderTimouet,
		WriteTimeout:      ServerWriteTimeout,
		IdleTimeout:       ServerIdleTimeout,
	}
}
