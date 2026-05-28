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
	IdempotencyKey *string
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
	Product         Product          `json:"product"`
	ProductVariants []ProductVariant `json:"productVariants"`
	ProductAssets   []ProductAsset   `json:"productAssets"`
}

func (q *Queries) skipProductSearchDocumentRefresh(ctx context.Context) error {
	_, err := q.db.Exec(ctx, "SELECT set_config('app.skip_search_doc_refresh', 'true', true)")
	return err
}

//nolint:gocognit
func (store *SQLStore) CreateProductTx(
	ctx context.Context,
	arg CreateProductTxParams,
) (CreateProductTxResults, error) {
	var results CreateProductTxResults

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		if err = q.skipProductSearchDocumentRefresh(ctx); err != nil {
			return err
		}

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

		if err = q.UpsertProductSearchDocument(ctx, results.Product.ID); err != nil {
			return err
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
		IdempotencyKey: arg.IdempotencyKey,
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

type UpdateProductTxParams struct {
	ProductID      uuid.UUID
	OrganizationID uuid.UUID
	CategoryID     uuid.UUID
	Name           string
	Slug           string
	Description    []byte
	Specification  []byte
	Status         string
	IsFeatured     bool
	Variants       []ProductVariantParams
	Assets         []ProductAssetParams
}

// UpdateProductTx updates an existing product, updates/creates its variants, deactivates omitted variants,
// and updates its assets.
func (store *SQLStore) UpdateProductTx(
	ctx context.Context,
	arg UpdateProductTxParams,
) (CreateProductTxResults, error) {
	var results CreateProductTxResults

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		if err = q.skipProductSearchDocumentRefresh(ctx); err != nil {
			return err
		}

		// 1. Update the product metadata.
		results.Product, err = q.UpdateProduct(ctx, UpdateProductParams{
			ID:             arg.ProductID,
			OrganizationID: arg.OrganizationID,
			CategoryID:     arg.CategoryID,
			Name:           arg.Name,
			Slug:           arg.Slug,
			Description:    arg.Description,
			Specification:  arg.Specification,
			Status:         arg.Status,
			IsFeatured:     arg.IsFeatured,
		})
		if err != nil {
			return err
		}

		// 2. Fetch existing variants of the product.
		existingVariants, err := q.ListProductVariantsByProductID(ctx, arg.ProductID)
		if err != nil {
			return err
		}

		// 3. Map existing variants by SKU.
		existingVariantsMap := make(map[string]ProductVariant, len(existingVariants))
		for _, v := range existingVariants {
			existingVariantsMap[v.Sku] = v
		}

		retainedSKUs := make(map[string]struct{}, len(arg.Variants))
		variantMapByID := make(map[string]uuid.UUID)

		// 4. Update or Create variants.
		for _, variantArg := range arg.Variants {
			var productVariant ProductVariant
			var variantErr error

			if existing, exists := existingVariantsMap[variantArg.Sku]; exists {
				// Update existing variant.
				productVariant, variantErr = q.UpdateProductVariant(ctx, UpdateProductVariantParams{
					ID:             existing.ID,
					OrganizationID: arg.OrganizationID,
					Name:           variantArg.Name,
					Price:          variantArg.Price,
				})
				if variantErr != nil {
					return variantErr
				}

				// Wipe existing attributes of the variant.
				if err = q.DeleteVariantAttributes(ctx, productVariant.ID); err != nil {
					return err
				}
			} else {
				// Create new variant.
				productVariant, variantErr = q.CreateProductVariant(
					ctx,
					buildCreateProductVariantParams(
						arg.OrganizationID,
						results.Product.ID,
						variantArg,
					),
				)
				if variantErr != nil {
					return variantErr
				}
			}

			// Assign attributes.
			for _, attrValueID := range variantArg.AttributeValueIDs {
				if err = q.AssignAttributeValueToProductVariant(
					ctx,
					AssignAttributeValueToProductVariantParams{
						ProductVariantID: productVariant.ID,
						AttributeValueID: attrValueID,
					},
				); err != nil {
					return err
				}
			}

			results.ProductVariants = append(results.ProductVariants, productVariant)
			retainedSKUs[variantArg.Sku] = struct{}{}
			variantMapByID[variantArg.Sku] = productVariant.ID
		}

		// 5. Deactivate variants not in incoming request.
		for sku, existing := range existingVariantsMap {
			if _, retained := retainedSKUs[sku]; !retained {
				if err = q.DeleteProductVariant(ctx, DeleteProductVariantParams{
					ID:             existing.ID,
					OrganizationID: arg.OrganizationID,
				}); err != nil {
					return err
				}
			}
		}

		// 6. Replace all existing assets for the product.
		if err = q.DeleteProductAssetsByProductID(ctx, results.Product.ID); err != nil {
			return err
		}

		// 7. Insert new product assets.
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

		// 8. Insert variant assets.
		for _, variantArg := range arg.Variants {
			if variantArg.Asset != nil {
				variantID := variantMapByID[variantArg.Sku]
				createdAsset, assetErr := q.CreateProductAsset(
					ctx,
					buildCreateProductAssetParams(
						*variantArg.Asset,
						results.Product.ID,
						&variantID,
					),
				)
				if assetErr != nil {
					return assetErr
				}
				results.ProductAssets = append(results.ProductAssets, createdAsset)
			}
		}

		if err = q.UpsertProductSearchDocument(ctx, results.Product.ID); err != nil {
			return err
		}

		return nil
	})

	return results, err
}
