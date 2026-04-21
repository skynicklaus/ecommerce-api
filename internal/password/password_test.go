package password_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skynicklaus/ecommerce-api/internal/password"
	"github.com/skynicklaus/ecommerce-api/util"
)

func TestPassword(t *testing.T) {
	randomString1 := util.GetRandomString(t, 8)
	require.NotEmpty(t, randomString1)

	hashedPassword1, err := password.HashPassword(randomString1)
	require.NoError(t, err)
	require.NotEmpty(t, hashedPassword1)

	err = password.CheckPassword(randomString1, hashedPassword1)
	require.NoError(t, err)

	randomString2 := util.GetRandomString(t, 6)
	require.NotEmpty(t, randomString2)

	err = password.CheckPassword(randomString2, hashedPassword1)
	require.EqualError(t, err, password.ErrMismatchedHashAndPassword.Error())

	hashedPassword2, err := password.HashPassword(randomString1)
	require.NoError(t, err)
	require.NotEmpty(t, hashedPassword2)
	require.NotEqual(t, hashedPassword1, hashedPassword2)
}
