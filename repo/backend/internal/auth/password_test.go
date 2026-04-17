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
		{"exactly ten", "abcdefghij", nil},
		{"long enough", strings.Repeat("x", 64), nil},
		{"unicode ten runes", "π€αβγδεζηθ", nil},
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
	pw := "correcthorsebatterystaple"
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
	hash, err := HashPassword("correcthorsebattery")
	if err != nil {
		t.Fatal(err)
	}
	if err := ComparePassword(hash, "correcthorsebatteryX"); err != ErrMismatched {
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
