package middleware

import (
	"golang.org/x/sync/singleflight"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
)

type Middleware struct {
	store    db.Store
	renewalG singleflight.Group
}

func New(store db.Store) *Middleware {
	return &Middleware{
		store:    store,
		renewalG: singleflight.Group{},
	}
}
