package db

import (
	"context"

	"github.com/google/uuid"
)

type CreateWarehouseTxParams struct {
	OrganizationID uuid.UUID
	Name           string
	Address        CreateAddressParams
}

type UpdateWarehouseTxParams struct {
	ID             int64
	OrganizationID uuid.UUID
	Name           string
	IsActive       bool
	Address        UpdateAddressByIDAndOrganizationParams
}

type WarehouseTxResult struct {
	Warehouse Warehouse
	Address   Address
}

func (store *SQLStore) CreateWarehouseTx(
	ctx context.Context,
	arg CreateWarehouseTxParams,
) (WarehouseTxResult, error) {
	var result WarehouseTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		arg.Address.OrganizationID = arg.OrganizationID
		result.Address, err = q.CreateAddress(ctx, arg.Address)
		if err != nil {
			return err
		}

		result.Warehouse, err = q.CreateWarehouse(ctx, CreateWarehouseParams{
			OrganizationID: arg.OrganizationID,
			Name:           arg.Name,
			AddressID:      result.Address.ID,
		})
		return err
	})

	return result, err
}

func (store *SQLStore) UpdateWarehouseTx(
	ctx context.Context,
	arg UpdateWarehouseTxParams,
) (WarehouseTxResult, error) {
	var result WarehouseTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		current, err := q.GetWarehouseByIDAndOrganization(
			ctx,
			GetWarehouseByIDAndOrganizationParams{
				ID:             arg.ID,
				OrganizationID: arg.OrganizationID,
			},
		)
		if err != nil {
			return err
		}

		result.Warehouse, err = q.UpdateWarehouse(ctx, UpdateWarehouseParams{
			ID:             arg.ID,
			OrganizationID: arg.OrganizationID,
			Name:           arg.Name,
			IsActive:       arg.IsActive,
		})
		if err != nil {
			return err
		}

		arg.Address.ID = current.AddressID
		arg.Address.OrganizationID = arg.OrganizationID
		result.Address, err = q.UpdateAddressByIDAndOrganization(ctx, arg.Address)
		return err
	})

	return result, err
}
