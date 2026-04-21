package cache

import (
	"context"
	"log/slog"
	"os"

	"github.com/redis/go-redis/v9"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

type RedisClient struct {
	*redis.Client

	store  db.Store
	logger *util.ServerLogger
}

func NewRedis(store db.Store, logger *util.ServerLogger) *RedisClient {
	redisURL, exists := os.LookupEnv("REDIS_URL")

	var redisClient *redis.Client
	if !exists || redisURL == "" {
		redisAddr, addrExists := os.LookupEnv("REDIS_ADDR")
		if !addrExists {
			redisAddr = "localhost:6379"
		}

		//nolint:exhaustruct // too many unnecessary fields
		redisClient = redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: "",
			DB:       0,
		})
	} else {
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			logger.Fatal("error parsing  redis URL", slog.Any("err", err))
		}

		redisClient = redis.NewClient(opts)
	}

	_, err := redisClient.Ping(context.Background()).Result()
	if err != nil {
		logger.Fatal("error connecting to redis", slog.Any("err", err))
	}

	return &RedisClient{
		Client: redisClient,
		store:  store,
		logger: logger,
	}
}
