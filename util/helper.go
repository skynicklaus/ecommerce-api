package util

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// HashSessionToken returns the lowercase hex-encoded SHA-256 of rawToken.
// Store the hash in the DB; give rawToken to the client once.
func HashSessionToken(rawToken string) string {
	h := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(h[:])
}

func DeferString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func DeferTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}

	return *t
}
