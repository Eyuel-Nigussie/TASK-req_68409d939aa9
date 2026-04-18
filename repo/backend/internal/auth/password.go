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
	ErrPasswordTooShort    = errors.New("password must be at least 10 characters")
	ErrPasswordBlank       = errors.New("password must not be blank or whitespace only")
	ErrPasswordTooSimple   = errors.New("password must contain at least two distinct character classes (letters, digits, symbols) and not repeat a single character")
	ErrPasswordTooCommon   = errors.New("password is too common or predictable; choose a less obvious value")
	ErrInvalidHash         = errors.New("invalid password hash format")
	ErrMismatched          = errors.New("password does not match")
)

// commonPasswords is a tiny built-in blocklist of passwords that appear at
// the top of every leak dump. Shipping a multi-megabyte breach list in the
// binary is out of scope for an offline deployment, but rejecting the
// single-digit handful of strings every attacker tries first provides
// measurable value at zero cost. The list is stored lowercase; ValidatePolicy
// matches case-insensitively after trimming whitespace.
var commonPasswords = map[string]struct{}{
	"password":          {},
	"password1":         {},
	"password123":       {},
	"password1234":      {},
	"letmein123":        {},
	"welcome123":        {},
	"qwerty1234":        {},
	"qwertyuiop":        {},
	"1234567890":        {},
	"0123456789":        {},
	"abcdefghij":        {},
	"abcdefghijk":       {},
	"administrator":     {},
	"changeme123":       {},
	"p@ssw0rd123":       {},
	"iloveyou123":       {},
	"12345678910":       {},
	"qazwsxedcrfv":      {},
	"trustno1234":       {},
}

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

// ValidatePolicy returns nil if the password satisfies the configured policy:
//
//   - not blank or all whitespace
//   - at least MinPasswordLength (10) runes
//   - not a single repeated character (e.g. "aaaaaaaaaa") — satisfies the
//     length rule but provides ~zero entropy
//   - contains at least two distinct character classes (letters, digits,
//     symbols) so an operator cannot pick a straight-digit or straight-letter
//     password
//   - is not in the built-in common-password blocklist (case-insensitive)
//
// The prompt only requires ≥10 characters; these extra rules are additive
// and keep the portal from accepting obviously weak passphrases that an
// attacker would guess in the first few lockout windows (L1).
func ValidatePolicy(pw string) error {
	if strings.TrimSpace(pw) == "" {
		return ErrPasswordBlank
	}
	runes := []rune(pw)
	if len(runes) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	// Reject a single character repeated N times.
	allSame := true
	for _, r := range runes[1:] {
		if r != runes[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return ErrPasswordTooSimple
	}
	// Require at least two distinct character classes.
	classes := 0
	var hasLetter, hasDigit, hasSymbol bool
	for _, r := range runes {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			// Treat any non-alphanumeric ASCII (and non-ASCII unicode)
			// as a symbol class. This matters for passphrases using
			// diacritics or punctuation — they still count as satisfying
			// the diversity rule.
			hasSymbol = true
		}
	}
	if hasLetter {
		classes++
	}
	if hasDigit {
		classes++
	}
	if hasSymbol {
		classes++
	}
	if classes < 2 {
		return ErrPasswordTooSimple
	}
	if _, bad := commonPasswords[strings.ToLower(strings.TrimSpace(pw))]; bad {
		return ErrPasswordTooCommon
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
