package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	db "github.com/skynicklaus/ecommerce-api/db/sqlc"
	"github.com/skynicklaus/ecommerce-api/util"
)

const (
	SystemPlatformRolesCacheKey   = "roles.system.platform"
	SystemMerchantRolesCacheKey   = "roles.system.merchant"
	SystemIndividualRolesCacheKey = "roles.system.individual"
	SystemCompanyRolesCacheKey    = "roles.system.company"
	RoleCacheDuation              = 15 * time.Minute
)

func (c *RedisClient) getRoleFromCacheOrDB(
	ctx context.Context,
	orgType string,
	slug string,
) (db.Role, error) {
	key := orgType + ":" + slug

	// 1. Thread-safe RLock check
	c.roleMu.RLock()
	role, found := c.roleMap[key]
	c.roleMu.RUnlock()
	if found {
		return role, nil
	}

	// 2. Fetch all system roles for this org type (with JSON caching)
	roles, err := getRolesJSONCache(ctx, c.logger, c.store, c.Client, orgType)
	if err != nil {
		return db.Role{}, err
	}

	// 3. Thread-safe Write-Lock populate
	c.roleMu.Lock()
	var targetRole db.Role
	var foundTarget bool
	for _, r := range roles {
		c.roleMap[orgType+":"+r.Slug] = r
		if r.Slug == slug {
			targetRole = r
			foundTarget = true
		}
	}
	c.roleMu.Unlock()

	if foundTarget {
		return targetRole, nil
	}
	return db.Role{}, ErrRoleNotFound
}

func (c *RedisClient) GetSystemPlatformRoleFromSlug(
	ctx context.Context,
	slug string,
) (db.Role, error) {
	return c.getRoleFromCacheOrDB(ctx, string(util.OrganizationTypePlatform), slug)
}

func (c *RedisClient) GetSystemMerchantRoleFromSlug(
	ctx context.Context,
	slug string,
) (db.Role, error) {
	return c.getRoleFromCacheOrDB(ctx, string(util.OrganizationTypeMerchant), slug)
}

func (c *RedisClient) GetSystemIndividualRoleFromSlug(
	ctx context.Context,
	slug string,
) (db.Role, error) {
	return c.getRoleFromCacheOrDB(ctx, string(util.OrganizationTypeIndividual), slug)
}

func (c *RedisClient) GetSystemCompanyRoleFromSlug(
	ctx context.Context,
	slug string,
) (db.Role, error) {
	return c.getRoleFromCacheOrDB(ctx, string(util.OrganizationTypeCompany), slug)
}

func getRolesJSONCache(
	ctx context.Context,
	logger *util.ServerLogger,
	store db.Store,
	client *redis.Client,
	organizationType string,
) ([]db.Role, error) {
	var cacheKey string
	switch organizationType {
	case string(util.OrganizationTypePlatform):
		cacheKey = SystemPlatformRolesCacheKey
	case string(util.OrganizationTypeMerchant):
		cacheKey = SystemMerchantRolesCacheKey
	case string(util.OrganizationTypeIndividual):
		cacheKey = SystemIndividualRolesCacheKey
	case string(util.OrganizationTypeCompany):
		cacheKey = SystemCompanyRolesCacheKey
	default:
		return nil, errors.New("invalid organizationType")
	}

	if roles, err := getRolesJSONCacheData(ctx, client, cacheKey); err == nil {
		return roles, nil
	}

	roles, err := store.ListOrganizationRolesByType(ctx, db.ListOrganizationRolesByTypeParams{
		OrganizationID:   nil,
		OrganizationType: organizationType,
	})
	if err != nil {
		return nil, err
	}

	if cacheErr := cacheRoles(ctx, client, cacheKey, roles); cacheErr != nil {
		logger.WarnContext(ctx, "failed to cache roles", "err", cacheErr)
	}

	return roles, nil
}

func getRolesJSONCacheData(
	ctx context.Context,
	client *redis.Client,
	cacheKey string,
) ([]db.Role, error) {
	jsonData, err := client.Get(ctx, cacheKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, errCacheMiss
		}
		return nil, err
	}

	var roles []db.Role
	if jsonErr := json.Unmarshal(jsonData, &roles); jsonErr != nil {
		client.Del(ctx, cacheKey)
		return nil, jsonErr
	}

	return roles, nil
}

func cacheRoles(
	ctx context.Context,
	client *redis.Client,
	cacheKey string,
	roles []db.Role,
) error {
	jsonData, err := json.Marshal(roles)
	if err != nil {
		return err
	}
	return client.Set(ctx, cacheKey, jsonData, RoleCacheDuation).Err()
}
