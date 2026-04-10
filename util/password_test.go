package util_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skynicklaus/ecommerce-api/util"
)

func TestPassword(t *testing.T) {
	password := util.GetRandomString(t, 8)
	require.NotEmpty(t, password)

	hashedPassword1, err := util.HashPassword(password)
	require.NoError(t, err)
	require.NotEmpty(t, hashedPassword1)

	err = util.CheckPassword(password, hashedPassword1)
	require.NoError(t, err)

	wrongPassword := util.GetRandomString(t, 6)
	require.NotEmpty(t, wrongPassword)

	err = util.CheckPassword(wrongPassword, hashedPassword1)
	require.EqualError(t, err, util.ErrMismatchedHashAndPassword.Error())

	hashedPassword2, err := util.HashPassword(password)
	require.NoError(t, err)
	require.NotEmpty(t, hashedPassword2)
	require.NotEqual(t, hashedPassword1, hashedPassword2)
}
