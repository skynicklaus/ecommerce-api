package middleware

import (
	"golang.org/x/sync/singleflight"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
)

type Middleware struct {
	store    db.Store
	cache    *cache.RedisClient
	renewalG singleflight.Group
}

func New(store db.Store, cache *cache.RedisClient) *Middleware {
	return &Middleware{
		store:    store,
		cache:    cache,
		renewalG: singleflight.Group{},
	}
}
