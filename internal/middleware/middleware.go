package middleware

import (
	"sync"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/internal/cache"
)

type Middleware struct {
	store            db.Store
	cache            *cache.Client
	renewalMu        sync.Mutex
	renewalsInFlight map[string]struct{}
}

func New(store db.Store, cache *cache.Client) *Middleware {
	return &Middleware{
		store:            store,
		cache:            cache,
		renewalsInFlight: make(map[string]struct{}),
	}
}

func (m *Middleware) beginSessionRenewal(tokenHash string) bool {
	m.renewalMu.Lock()
	defer m.renewalMu.Unlock()

	if _, exists := m.renewalsInFlight[tokenHash]; exists {
		return false
	}
	m.renewalsInFlight[tokenHash] = struct{}{}
	return true
}

func (m *Middleware) finishSessionRenewal(tokenHash string) {
	m.renewalMu.Lock()
	delete(m.renewalsInFlight, tokenHash)
	m.renewalMu.Unlock()
}
