package cache

import (
	"context"
	"fmt"
	"time"
)

const (
	PendingUploadCacheKey = "upload:"
	maxPendingUploadTTL   = 12 * time.Hour
)

func (c *RedisClient) CachePendingUpload(
	ctx context.Context,
	token string,
	data []byte,
) error {
	key := fmt.Sprintf("%s%s", PendingUploadCacheKey, token)
	_, err := c.Set(ctx, key, data, maxPendingUploadTTL).Result()

	return err
}
