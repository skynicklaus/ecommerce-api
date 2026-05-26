package cache

import (
	"context"
	"time"
)

const presignedURLPrefix = "presigned:"

// PresignedURLKey is the cache key used to memoize a presigned S3 GET URL for a
// given final asset key. Exported so tests can construct the same key.
func PresignedURLKey(assetKey string) string {
	return presignedURLPrefix + assetKey
}

func (c *Client) GetPresignedURL(ctx context.Context, assetKey string) (string, error) {
	return c.Get(ctx, PresignedURLKey(assetKey)).Result()
}

func (c *Client) CachePresignedURL(
	ctx context.Context,
	assetKey, url string,
	ttl time.Duration,
) error {
	return c.Set(ctx, PresignedURLKey(assetKey), url, ttl).Err()
}
