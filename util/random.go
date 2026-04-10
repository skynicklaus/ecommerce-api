//nolint:mnd,gosec // test helpers
package util

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	mathRand "math/rand/v2"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func GetRandomIdentityType(t *testing.T) string {
	t.Helper()

	identityTypes := []string{"user", "customer"}

	n := big.NewInt(int64(len(identityTypes)))

	randomIndex, err := rand.Int(rand.Reader, n)
	if err != nil {
		t.Fatalf("failed to get random identity type: %v\n", err)
	}

	return identityTypes[randomIndex.Int64()]
}

func GetRandomString(t *testing.T, length int) string {
	t.Helper()

	alphabet := "abcdefghijklmnopqrstuvxyz"

	var sb strings.Builder
	n := big.NewInt(int64(len(alphabet)))

	for range length {
		randomIndex, err := rand.Int(rand.Reader, n)
		if err != nil {
			t.Fatalf("failed to get random string: %v\n", err)
		}
		sb.WriteByte(alphabet[randomIndex.Int64()])
	}

	return sb.String()
}

func GetRandomStringPtr(t *testing.T, length int) *string {
	randomString := GetRandomString(t, length)

	return &randomString
}

func GetRandomEmail(t *testing.T, length int) string {
	t.Helper()

	randomString := GetRandomString(t, length)

	domains := []string{
		"example.com",
		"test.org",
		"mail.net",
		"domain.io",
	} // Add more domains as needed

	n := big.NewInt(int64(len(domains)))
	randomIndex, err := rand.Int(rand.Reader, n)
	if err != nil {
		t.Fatalf("failed to get random domain: %v\n", err)
	}
	domain := domains[randomIndex.Int64()]

	return fmt.Sprintf("%s@%s", randomString, domain)
}

func GetRandomProvider(t *testing.T) string {
	t.Helper()

	providers := []string{
		"credential",
		"google",
	}

	randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(providers))))
	if err != nil {
		t.Fatalf("failed to get random provider: %v\n", err)
	}

	return providers[randomIndex.Int64()]
}

func GetRandomOrganizationType(t *testing.T) string {
	t.Helper()

	organizaionTypes := []string{
		"platform",
		"merchant",
		"individual",
		"company",
	}

	randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(organizaionTypes))))
	if err != nil {
		t.Fatalf("failed to get random organization type: %v\n", err)
	}

	return organizaionTypes[randomIndex.Int64()]
}

func GetRandomOrganizationStatus(t *testing.T) string {
	t.Helper()

	organizationStatus := []string{
		"active",
		"pending",
		"suspended",
	}

	randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(organizationStatus))))
	if err != nil {
		t.Fatalf("failed to get random organization status: %v\n", err)
	}

	return organizationStatus[randomIndex.Int64()]
}

func GetRandomHashedPassword(t *testing.T, length int) *string {
	t.Helper()

	hashedPassword, err := HashPassword(GetRandomString(t, length))
	if err != nil {
		t.Fatalf("failed to hash password: %v\n", err)
	}

	return &hashedPassword
}

func GetRandomAddressType(t *testing.T) string {
	t.Helper()

	addressTypes := []string{
		"shipping",
		"billing",
		"warehouse",
		"general",
	}

	randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(addressTypes))))
	if err != nil {
		t.Fatalf("unable to get random address type: %v", err)
	}

	return addressTypes[randomIndex.Int64()]
}

func GetRandomNumberString(t *testing.T, length int) string {
	t.Helper()

	alphabet := "0123456789"

	var sb strings.Builder
	n := big.NewInt(int64(len(alphabet)))

	for range length {
		randomIndex, err := rand.Int(rand.Reader, n)
		if err != nil {
			t.Fatalf("failed to get random string: %v\n", err)
		}
		sb.WriteByte(alphabet[randomIndex.Int64()])
	}

	return sb.String()
}

func CoinFlip(t *testing.T) int64 {
	t.Helper()

	n, err := rand.Int(rand.Reader, big.NewInt(2))
	if err != nil {
		t.Fatalf("failed coin flip: %v", err)
	}

	return n.Int64()
}

func GetRandomSortOrder(t *testing.T) int16 {
	t.Helper()

	n, err := rand.Int(rand.Reader, big.NewInt(20))
	if err != nil {
		t.Fatalf("failed to get random sort order: %v", err)
	}

	return int16((n.Int64() + 1) * 10)
}

func GetRandomDescriptionJSON(t *testing.T, length int) []byte {
	t.Helper()

	json, err := json.Marshal(struct {
		Description string `json:"description"`
	}{
		Description: GetRandomString(t, length),
	})
	if err != nil {
		t.Fatalf("failed to generate description json: %v", err)
	}

	return json
}

func GetRandomPrice() decimal.Decimal {
	minPrice := 10.00
	maxPrice := 100.00

	randomDecimalInRange := mathRand.Float64()*(maxPrice-minPrice) + minPrice

	return decimal.NewFromFloatWithExponent(randomDecimalInRange, int32(-2))
}

func GetRandomNumber(t *testing.T, maxLimit int) int32 {
	t.Helper()

	n, err := rand.Int(rand.Reader, big.NewInt(int64(maxLimit)))
	if err != nil {
		t.Fatalf("failed to generate random number: %v", err)
	}

	return int32(n.Int64()+1) * 10
}

func GetRandomNumberPtr(t *testing.T, maxLimit int) *int32 {
	t.Helper()

	randomNumber := GetRandomNumber(t, maxLimit)

	return &randomNumber
}
