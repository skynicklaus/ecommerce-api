package db

import (
	"context"

	"github.com/google/uuid"
)

type AddCartItemTxParams struct {
	BuyerOrgID       uuid.UUID
	ProductVariantID uuid.UUID
	Quantity         int16
}

type AddCartItemTxResult struct {
	Cart         Cart
	ShopGroup    CartShopGroup
	Item         CartItem
	Variant      GetActiveVariantForCartRow
	UpdatedGroup CartShopGroup
}

func (store *SQLStore) AddCartItemTx(
	ctx context.Context,
	arg AddCartItemTxParams,
) (AddCartItemTxResult, error) {
	var result AddCartItemTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		result.Cart, err = q.CreateCart(ctx, arg.BuyerOrgID)
		if err != nil {
			return err
		}

		result.Variant, err = q.GetActiveVariantForCart(ctx, arg.ProductVariantID)
		if err != nil {
			return err
		}

		result.ShopGroup, err = q.GetOrCreateCartShopGroup(ctx, GetOrCreateCartShopGroupParams{
			CartID:        result.Cart.ID,
			MerchantOrgID: result.Variant.MerchantOrgID,
		})
		if err != nil {
			return err
		}

		result.Item, err = q.UpsertCartItem(ctx, UpsertCartItemParams{
			CartShopGroupID:  result.ShopGroup.ID,
			ProductVariantID: arg.ProductVariantID,
			Quantity:         arg.Quantity,
			UnitPrice:        result.Variant.Price,
		})
		if err != nil {
			return err
		}

		if _, err = q.RecalculateCartShopGroupSubtotal(ctx, result.ShopGroup.ID); err != nil {
			return err
		}

		result.UpdatedGroup, err = q.RecalculateCartShopGroupSelection(ctx, result.ShopGroup.ID)
		if err != nil {
			return err
		}

		return nil
	})

	return result, err
}

type UpdateCartItemQuantityTxParams struct {
	BuyerOrgID uuid.UUID
	CartItemID uuid.UUID
	Quantity   int16
}

type UpdateCartItemQuantityTxResult struct {
	Item         CartItem
	UpdatedGroup CartShopGroup
}

func (store *SQLStore) UpdateCartItemQuantityTx(
	ctx context.Context,
	arg UpdateCartItemQuantityTxParams,
) (UpdateCartItemQuantityTxResult, error) {
	var result UpdateCartItemQuantityTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		result.Item, err = q.UpdateCartItemQuantityForBuyerOrg(ctx, UpdateCartItemQuantityForBuyerOrgParams{
			CartItemID: arg.CartItemID,
			BuyerOrgID: arg.BuyerOrgID,
			Quantity:   arg.Quantity,
		})
		if err != nil {
			return err
		}

		result.UpdatedGroup, err = q.RecalculateCartShopGroupSubtotal(ctx, result.Item.CartShopGroupID)
		if err != nil {
			return err
		}

		return nil
	})

	return result, err
}

type SetCartItemSelectedTxParams struct {
	BuyerOrgID uuid.UUID
	CartItemID uuid.UUID
	IsSelected bool
}

type SetCartItemSelectedTxResult struct {
	Item         CartItem
	UpdatedGroup CartShopGroup
}

func (store *SQLStore) SetCartItemSelectedTx(
	ctx context.Context,
	arg SetCartItemSelectedTxParams,
) (SetCartItemSelectedTxResult, error) {
	var result SetCartItemSelectedTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		result.Item, err = q.SetCartItemSelectedForBuyerOrg(ctx, SetCartItemSelectedForBuyerOrgParams{
			CartItemID: arg.CartItemID,
			BuyerOrgID: arg.BuyerOrgID,
			IsSelected: arg.IsSelected,
		})
		if err != nil {
			return err
		}

		result.UpdatedGroup, err = q.RecalculateCartShopGroupSelection(ctx, result.Item.CartShopGroupID)
		if err != nil {
			return err
		}

		return nil
	})

	return result, err
}

type RemoveCartItemTxParams struct {
	BuyerOrgID uuid.UUID
	CartItemID uuid.UUID
}

func (store *SQLStore) RemoveCartItemTx(ctx context.Context, arg RemoveCartItemTxParams) error {
	return store.execTx(ctx, func(q *Queries) error {
		item, err := q.GetCartItemForBuyerOrg(ctx, GetCartItemForBuyerOrgParams{
			CartItemID: arg.CartItemID,
			BuyerOrgID: arg.BuyerOrgID,
		})
		if err != nil {
			return err
		}

		if err = q.DeleteCartItemForBuyerOrg(ctx, DeleteCartItemForBuyerOrgParams{
			CartItemID: arg.CartItemID,
			BuyerOrgID: arg.BuyerOrgID,
		}); err != nil {
			return err
		}

		if _, err = q.RecalculateCartShopGroupSubtotal(ctx, item.CartShopGroupID); err != nil {
			return err
		}
		if _, err = q.RecalculateCartShopGroupSelection(ctx, item.CartShopGroupID); err != nil {
			return err
		}

		return q.DeleteEmptyCartShopGroups(ctx, item.CartID)
	})
}

type SetCartShopGroupSelectedTxParams struct {
	BuyerOrgID      uuid.UUID
	CartShopGroupID uuid.UUID
	IsSelected      bool
}

type SetCartShopGroupSelectedTxResult struct {
	ShopGroup CartShopGroup
}

func (store *SQLStore) SetCartShopGroupSelectedTx(
	ctx context.Context,
	arg SetCartShopGroupSelectedTxParams,
) (SetCartShopGroupSelectedTxResult, error) {
	var result SetCartShopGroupSelectedTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		if _, err = q.SetCartShopGroupSelectedForBuyerOrg(
			ctx,
			SetCartShopGroupSelectedForBuyerOrgParams{
				CartShopGroupID: arg.CartShopGroupID,
				BuyerOrgID:      arg.BuyerOrgID,
				IsSelected:      arg.IsSelected,
			},
		); err != nil {
			return err
		}

		if err = q.SetCartItemsSelectedByGroupForBuyerOrg(ctx, SetCartItemsSelectedByGroupForBuyerOrgParams{
			CartShopGroupID: arg.CartShopGroupID,
			BuyerOrgID:      arg.BuyerOrgID,
			IsSelected:      arg.IsSelected,
		}); err != nil {
			return err
		}

		result.ShopGroup, err = q.RecalculateCartShopGroupSelection(ctx, arg.CartShopGroupID)
		if err != nil {
			return err
		}

		return nil
	})

	return result, err
}
