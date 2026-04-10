package util

import (
	"errors"
	"fmt"

	"github.com/alexedwards/argon2id"
)

var ErrMismatchedHashAndPassword = errors.New(
	"hashedPassword is not the hash of the given password",
)

func HashPassword(password string) (string, error) {
	hashedPassword, err := argon2id.CreateHash(password, argon2id.DefaultParams)
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
