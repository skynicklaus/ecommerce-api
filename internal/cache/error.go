package cache

import "errors"

var (
	errCacheMiss    = errors.New("cache miss")
	ErrRoleNotFound = errors.New("role not found")
)
