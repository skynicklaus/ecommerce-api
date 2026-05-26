package cache

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

type Client struct {
	*redis.Client

	store  db.Store
	logger *util.ServerLogger

	roleMu  sync.RWMutex
	roleMap map[string]db.Role
}

func New(store db.Store, logger *util.ServerLogger) *Client {
	redisURL, exists := os.LookupEnv("REDIS_URL")

	var redisClient *redis.Client
	if !exists || redisURL == "" {
		redisAddr, addrExists := os.LookupEnv("REDIS_ADDR")
		if !addrExists {
			redisAddr = "localhost:6379"
		}

		//nolint:exhaustruct // too many unnecessary fields
		redisClient = redis.NewClient(&redis.Options{
			Addr:         redisAddr,
			Password:     "",
			DB:           0,
			PoolSize:     25,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		})
	} else {
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			logger.Fatal("error parsing  redis URL", slog.Any("err", err))
		}
		opts.PoolSize = 25
		opts.DialTimeout = 5 * time.Second
		opts.ReadTimeout = 3 * time.Second
		opts.WriteTimeout = 3 * time.Second

		redisClient = redis.NewClient(opts)
	}

	_, err := redisClient.Ping(context.Background()).Result()
	if err != nil {
		logger.Fatal("error connecting to redis", slog.Any("err", err))
	}

	return &Client{
		Client:  redisClient,
		store:   store,
		logger:  logger,
		roleMap: make(map[string]db.Role),
	}
}
