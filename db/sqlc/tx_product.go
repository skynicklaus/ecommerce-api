package db

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type CreateProductTxParams struct {
	OrganizationID uuid.UUID
	CategoryID     uuid.UUID
	Name           string
	Slug           string
	Description    []byte
	Specification  []byte
	Variants       []ProductVariantParams
	Assets         []ProductAssetParams
}

type ProductVariantParams struct {
	Sku               string
	Name              string
	Price             decimal.Decimal
	AttributeValueIDs []int64
	Asset             *ProductAssetParams
}

type ProductAssetParams struct {
	AssetKey        string
	Type            string
	MimeType        string
	AltText         *string
	SortOrder       int16
	IsPrimary       bool
	DurationSeconds *int16
}

type CreateProductTxResults struct {
	Product         Product
	ProductVariants []ProductVariant
	ProductAssets   []ProductAsset
}

func (store *SQLStore) CreateProductTx(
	ctx context.Context,
	arg CreateProductTxParams,
) (CreateProductTxResults, error) {
	var results CreateProductTxResults

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		results.Product, err = q.CreateProduct(ctx, buildCreateProductParams(arg))
		if err != nil {
			return err
		}

		for _, asset := range arg.Assets {
			createdAsset, assetErr := q.CreateProductAsset(
				ctx,
				buildCreateProductAssetParams(asset, results.Product.ID, nil),
			)
			if assetErr != nil {
				return assetErr
			}

			results.ProductAssets = append(results.ProductAssets, createdAsset)
		}

		for _, variant := range arg.Variants {
			productVariant, variantErr := q.CreateProductVariant(
				ctx,
				buildCreateProductVariantParams(arg.OrganizationID, results.Product.ID, variant),
			)
			if variantErr != nil {
				return variantErr
			}

			for _, attributeValueID := range variant.AttributeValueIDs {
				if err = q.AssignAttributeValueToProductVariant(
					ctx,
					AssignAttributeValueToProductVariantParams{
						ProductVariantID: productVariant.ID,
						AttributeValueID: attributeValueID,
					},
				); err != nil {
					return err
				}
			}

			results.ProductVariants = append(results.ProductVariants, productVariant)

			if variant.Asset != nil {
				asset, assetErr := q.CreateProductAsset(
					ctx,
					buildCreateProductAssetParams(
						*variant.Asset,
						results.Product.ID,
						&productVariant.ID,
					),
				)
				if assetErr != nil {
					return assetErr
				}

				results.ProductAssets = append(results.ProductAssets, asset)
			}
		}

		return nil
	})

	return results, err
}

func buildCreateProductParams(arg CreateProductTxParams) CreateProductParams {
	return CreateProductParams{
		CategoryID:     arg.CategoryID,
		OrganizationID: arg.OrganizationID,
		Name:           arg.Name,
		Slug:           arg.Slug,
		Description:    arg.Description,
		Specification:  arg.Specification,
	}
}

func buildCreateProductVariantParams(
	organizationID uuid.UUID,
	productID uuid.UUID,
	arg ProductVariantParams,
) CreateProductVariantParams {
	return CreateProductVariantParams{
		OrganizationID: organizationID,
		ProductID:      productID,
		Name:           arg.Name,
		Sku:            arg.Sku,
		Price:          arg.Price,
	}
}

func buildCreateProductAssetParams(
	arg ProductAssetParams,
	productID uuid.UUID,
	productVariantID *uuid.UUID,
) CreateProductAssetParams {
	return CreateProductAssetParams{
		ProductID:        productID,
		ProductVariantID: productVariantID,
		AssetKey:         arg.AssetKey,
		Type:             arg.Type,
		MimeType:         arg.MimeType,
		AltText:          arg.AltText,
		SortOrder:        arg.SortOrder,
		IsPrimary:        arg.IsPrimary,
		DurationSeconds:  arg.DurationSeconds,
	}
}
