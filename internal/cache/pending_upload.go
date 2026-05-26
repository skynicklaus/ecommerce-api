package cache

import (
	"context"
	"time"
)

const (
	PendingUploadCacheKey = "upload:"
	maxPendingUploadTTL   = 12 * time.Hour
)

func (c *Client) CachePendingUpload(
	ctx context.Context,
	token string,
	data []byte,
) error {
	key := PendingUploadCacheKey + token
	_, err := c.Set(ctx, key, data, maxPendingUploadTTL).Result()

	return err
}

func (c *Client) GetPendingUpload(
	ctx context.Context,
	token string,
) ([]byte, error) {
	key := PendingUploadCacheKey + token
	return c.Get(ctx, key).Bytes()
}

func (c *Client) DeletePendingUpload(
	ctx context.Context,
	token string,
) error {
	key := PendingUploadCacheKey + token
	_, err := c.Del(ctx, key).Result()
	return err
}
