package db

import (
	"context"

	"github.com/google/uuid"
)

type CreateOrganizationTxRequest struct {
	IdentityID           uuid.UUID
	ParentID             *uuid.UUID
	Name                 string
	Slug                 string
	Type                 string
	Status               string
	Metadata             []byte
	RoleID               int16
	RoleOrganizationType string
}

type CreateOrganizationTxResponse struct {
	Organization Organization
	Member       Member
}

func (store *SQLStore) CreateOrganizationTx(
	ctx context.Context,
	arg CreateOrganizationTxRequest,
) (CreateOrganizationTxResponse, error) {
	var result CreateOrganizationTxResponse

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		if arg.Type != arg.RoleOrganizationType {
			return ErrMismatchOrganizationType
		}

		result.Organization, err = q.CreateOrganization(ctx, CreateOrganizationParams{
			ParentID: arg.ParentID,
			Name:     arg.Name,
			Slug:     arg.Slug,
			Type:     arg.Type,
			Status:   arg.Status,
			Metadata: arg.Metadata,
		})
		if err != nil {
			return err
		}

		result.Member, err = q.CreateMember(ctx, CreateMemberParams{
			OrganizationID: result.Organization.ID,
			IdentityID:     arg.IdentityID,
		})
		if err != nil {
			return err
		}

		err = q.AssignRoleToMember(ctx, AssignRoleToMemberParams{
			MemberID:   result.Member.ID,
			RoleID:     arg.RoleID,
			AssignedBy: nil,
		})
		if err != nil {
			return err
		}

		return nil
	})

	return result, err
}
