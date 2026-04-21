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

func (c *RedisClient) GetSystemPlatformRoleFromSlug(
	ctx context.Context,
	slug string,
) (db.Role, error) {
	roles, err := getRolesJSONCache(
		ctx,
		c.logger,
		c.store,
		c.Client,
		string(util.OrganizationTypePlatform),
	)
	if err != nil {
		return db.Role{}, err
	}

	for _, role := range roles {
		if role.Slug == slug {
			return role, nil
		}
	}

	return db.Role{}, ErrRoleNotFound
}

func (c *RedisClient) GetSystemMerchantRoleFromSlug(
	ctx context.Context,
	slug string,
) (db.Role, error) {
	roles, err := getRolesJSONCache(
		ctx,
		c.logger,
		c.store,
		c.Client,
		string(util.OrganizationTypeMerchant),
	)
	if err != nil {
		return db.Role{}, err
	}

	for _, role := range roles {
		if role.Slug == slug {
			return role, nil
		}
	}

	return db.Role{}, ErrRoleNotFound
}

func (c *RedisClient) GetSystemIndividualRoleFromSlug(
	ctx context.Context,
	slug string,
) (db.Role, error) {
	roles, err := getRolesJSONCache(
		ctx,
		c.logger,
		c.store,
		c.Client,
		string(util.OrganizationTypeIndividual),
	)
	if err != nil {
		return db.Role{}, err
	}

	for _, role := range roles {
		if role.Slug == slug {
			return role, nil
		}
	}

	return db.Role{}, ErrRoleNotFound
}

func (c *RedisClient) GetSystemCompanyRoleFromSlug(
	ctx context.Context,
	slug string,
) (db.Role, error) {
	roles, err := getRolesJSONCache(
		ctx,
		c.logger,
		c.store,
		c.Client,
		string(util.OrganizationTypeCompany),
	)
	if err != nil {
		return db.Role{}, err
	}

	for _, role := range roles {
		if role.Slug == slug {
			return role, nil
		}
	}

	return db.Role{}, ErrRoleNotFound
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
