package middleware

import db "github.com/skynicklaus/ecommerce-api/db/sqlc"

type Middleware struct {
	store db.Store
}

func New(store db.Store) *Middleware {
	return &Middleware{
		store: store,
	}
}
