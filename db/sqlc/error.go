package db

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	ForeignKeyViolation = "23503"
	UniqueViolation     = "23505"
	CheckViolation      = "23514"
)

// pgx error
var (
	ErrNotFound        = pgx.ErrNoRows
	ErrUniqueViolation = &pgconn.PgError{
		Code: UniqueViolation,
	}
	ErrForeignKeyViolation = &pgconn.PgError{
		Code: ForeignKeyViolation,
	}
	ErrCheckViolation = &pgconn.PgError{
		Code: CheckViolation,
	}
)

func ErrorCode(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}

	return ""
}

// db custom error
var (
	ErrMismatchOrganizationType = errors.New("organization type mistmatch")
	ErrInvalidUserType          = errors.New("invalid user type")
)
