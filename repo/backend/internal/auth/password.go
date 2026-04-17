// Package auth handles local username/password authentication, lockout tracking,
// and session token management. No external identity providers are used because
// the portal runs on an isolated network.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// MinPasswordLength is the minimum password length required by policy.
const MinPasswordLength = 10

// Password policy errors.
var (
	ErrPasswordTooShort = errors.New("password must be at least 10 characters")
	ErrPasswordBlank    = errors.New("password must not be blank or whitespace only")
	ErrInvalidHash      = errors.New("invalid password hash format")
	ErrMismatched       = errors.New("password does not match")
)

// Argon2id parameters tuned for interactive logins on commodity hardware.
// Chosen to complete in ~50-150ms on the target deployment class while still
// providing substantial resistance to offline guessing attacks.
const (
	argonTime    uint32 = 2
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 2
	argonKeyLen  uint32 = 32
	saltLength          = 16
)

// ValidatePolicy returns nil if the password satisfies the configured policy.
func ValidatePolicy(pw string) error {
	if strings.TrimSpace(pw) == "" {
		return ErrPasswordBlank
	}
	if len([]rune(pw)) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	return nil
}

// HashPassword returns an encoded Argon2id hash including algorithm parameters
// so the format can evolve without migrating records.
func HashPassword(pw string) (string, error) {
	if err := ValidatePolicy(pw); err != nil {
		return "", err
	}
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
	return encoded, nil
}

// ComparePassword returns nil when pw matches the stored hash. All comparison
// is performed in constant time to avoid timing oracles.
func ComparePassword(encoded, pw string) error {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return ErrInvalidHash
	}
	var mem, tm uint32
	var par uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &tm, &par); err != nil {
		return ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return ErrInvalidHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return ErrInvalidHash
	}
	got := argon2.IDKey([]byte(pw), salt, tm, mem, par, uint32(len(want)))
	if subtle.ConstantTimeCompare(want, got) != 1 {
		return ErrMismatched
	}
	return nil
}
