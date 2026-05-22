package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	idempotencyResultPrefix = "idempotency:product:"
	idempotencyLockPrefix   = "idempotency:lock:product:"
)

// IdempotencyResultKey is the cache key used to store a successful idempotent
// CreateProduct response. Exported so tests can construct the same key.
func IdempotencyResultKey(organizationID uuid.UUID, idempotencyKey string) string {
	return fmt.Sprintf("%s%s:%s", idempotencyResultPrefix, organizationID.String(), idempotencyKey)
}

// IdempotencyLockKey is the cache key used as a short-lived lock for an
// in-flight idempotent request. Exported so tests can simulate a held lock.
func IdempotencyLockKey(organizationID uuid.UUID, idempotencyKey string) string {
	return fmt.Sprintf("%s%s:%s", idempotencyLockPrefix, organizationID.String(), idempotencyKey)
}

func (c *RedisClient) GetIdempotentProductResult(
	ctx context.Context,
	organizationID uuid.UUID,
	idempotencyKey string,
) ([]byte, error) {
	return c.Get(ctx, IdempotencyResultKey(organizationID, idempotencyKey)).Bytes()
}

func (c *RedisClient) CacheIdempotentProductResult(
	ctx context.Context,
	organizationID uuid.UUID,
	idempotencyKey string,
	value []byte,
	ttl time.Duration,
) error {
	return c.Set(ctx, IdempotencyResultKey(organizationID, idempotencyKey), value, ttl).Err()
}

func (c *RedisClient) AcquireIdempotencyLock(
	ctx context.Context,
	organizationID uuid.UUID,
	idempotencyKey string,
	ttl time.Duration,
) (bool, error) {
	return c.SetNX(ctx, IdempotencyLockKey(organizationID, idempotencyKey), "1", ttl).Result()
}

func (c *RedisClient) ReleaseIdempotencyLock(
	ctx context.Context,
	organizationID uuid.UUID,
	idempotencyKey string,
) error {
	return c.Del(ctx, IdempotencyLockKey(organizationID, idempotencyKey)).Err()
}
