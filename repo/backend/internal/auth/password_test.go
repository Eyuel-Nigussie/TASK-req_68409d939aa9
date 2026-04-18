package auth

import (
	"strings"
	"testing"
)

func TestValidatePolicy(t *testing.T) {
	cases := []struct {
		name string
		pw   string
		want error
	}{
		{"empty", "", ErrPasswordBlank},
		{"spaces only", "         ", ErrPasswordBlank},
		{"nine chars", "abcdefghi", ErrPasswordTooShort},
		// Length met but only one class (letters) — rejected by L1.
		{"ten letters one class", "abcdefghij", ErrPasswordTooSimple},
		{"long single class letters", "thequickbrownfox", ErrPasswordTooSimple},
		// Length met but single character repeated — rejected.
		{"repeated single char", strings.Repeat("x", 64), ErrPasswordTooSimple},
		// Unicode-only (all symbols in our classification) — rejected as
		// single class. The test exists to pin behavior; a realistic
		// operator password would combine classes.
		{"unicode ten runes one class", "π€αβγδεζηθ", ErrPasswordTooSimple},
		// Common-leak entries rejected even at length.
		{"common entry", "Password123", ErrPasswordTooCommon},
		// Happy paths: ≥2 classes and not in blocklist.
		{"letters plus digits", "Horse1Battery2", nil},
		{"letters plus symbols", "correct-horse-battery-staple", nil},
		{"three classes", "Admin-Demo-12!", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidatePolicy(tc.pw)
			if got != tc.want {
				t.Fatalf("ValidatePolicy(%q) = %v, want %v", tc.pw, got, tc.want)
			}
		})
	}
}

func TestHashAndCompare_RoundTrip(t *testing.T) {
	pw := "correct-horse-battery-staple"
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("hash missing argon2id prefix: %s", hash)
	}
	if err := ComparePassword(hash, pw); err != nil {
		t.Fatalf("ComparePassword should succeed: %v", err)
	}
}

func TestHashAndCompare_WrongPassword(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if err := ComparePassword(hash, "correct-horse-batteryX"); err != ErrMismatched {
		t.Fatalf("expected ErrMismatched, got %v", err)
	}
}

func TestHashPassword_RejectsShort(t *testing.T) {
	if _, err := HashPassword("short"); err != ErrPasswordTooShort {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestHashPassword_SaltVaries(t *testing.T) {
	a, _ := HashPassword("samepassword123")
	b, _ := HashPassword("samepassword123")
	if a == b {
		t.Fatalf("hashes of same password should differ due to salt")
	}
}

func TestComparePassword_InvalidHash(t *testing.T) {
	cases := []string{"", "not-a-hash", "$bcrypt$foo", "$argon2id$broken$$$"}
	for _, c := range cases {
		if err := ComparePassword(c, "whatever"); err != ErrInvalidHash {
			t.Errorf("ComparePassword(%q) expected ErrInvalidHash, got %v", c, err)
		}
	}
}
