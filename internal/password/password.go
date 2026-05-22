package password

import (
	"errors"
	"fmt"

	"github.com/alexedwards/argon2id"
)

// Argon2id parameters following OWASP minimum recommendation for production.
// DefaultParams (t=1) is documented by the library as development/testing only.
const (
	argon2Memory      = 128 * 1024 // 128 MB
	argon2Iterations  = 3
	argon2Parallelism = 4
	argon2SaltLength  = 16
	argon2KeyLength   = 32
)

var ErrMismatchedHashAndPassword = errors.New(
	"hashedPassword is not the hash of the given password",
)

func HashPassword(password string) (string, error) {
	params := &argon2id.Params{
		Memory:      argon2Memory,
		Iterations:  argon2Iterations,
		Parallelism: argon2Parallelism,
		SaltLength:  argon2SaltLength,
		KeyLength:   argon2KeyLength,
	}
	hashedPassword, err := argon2id.CreateHash(password, params)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	return hashedPassword, nil
}

func CheckPassword(password string, hashedPassword string) error {
	match, err := argon2id.ComparePasswordAndHash(password, hashedPassword)
	if err != nil {
		return fmt.Errorf("error while comparing password: %w", err)
	}

	if !match {
		return ErrMismatchedHashAndPassword
	}

	return nil
}
