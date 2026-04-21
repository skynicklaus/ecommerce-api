package server

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
	"github.com/skynicklaus/ecommerce-api/util"
)

const (
	ServerReadTimeout       = 10 * time.Second
	ServerReadHeaderTimouet = 10 * time.Second
	ServerWriteTimeout      = 20 * time.Second
	ServerIdleTimeout       = 120 * time.Second
)

type Server struct {
	port   int
	logger *util.ServerLogger
	store  db.Store
	redis  *cache.RedisClient
}

func NewServer(store db.Store, logger *util.ServerLogger) *http.Server {
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	if port == 0 {
		port = 8080
	}

	redis := cache.NewRedis(store, logger)

	newServer := &Server{
		port:   port,
		logger: logger,
		store:  store,
		redis:  redis,
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
