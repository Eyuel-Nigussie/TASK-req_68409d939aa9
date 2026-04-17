package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func newTestVault(t *testing.T, versions ...uint16) *Vault {
	t.Helper()
	keys := make(map[uint16][]byte)
	for _, v := range versions {
		k := make([]byte, 32)
		if _, err := rand.Read(k); err != nil {
			t.Fatal(err)
		}
		keys[v] = k
	}
	v, err := NewVault(keys)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestVault_RoundTrip(t *testing.T) {
	v := newTestVault(t, 1)
	for _, pt := range []string{
		"123-45-6789",
		"789 Any Street, Apt 4, Springfield, IL 62704",
		"",
		"unicode: π€日本語",
	} {
		ct, err := v.Encrypt(pt)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", pt, err)
		}
		got, err := v.Decrypt(ct)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if got != pt {
			t.Fatalf("round trip mismatch: %q -> %q", pt, got)
		}
	}
}

func TestVault_EmptyStaysEmpty(t *testing.T) {
	v := newTestVault(t, 1)
	ct, _ := v.Encrypt("")
	if ct != "" {
		t.Fatalf("expected empty envelope for empty input, got %q", ct)
	}
	pt, _ := v.Decrypt("")
	if pt != "" {
		t.Fatalf("expected empty plaintext for empty input, got %q", pt)
	}
}

func TestVault_CiphertextVariesAcrossCalls(t *testing.T) {
	v := newTestVault(t, 1)
	a, _ := v.Encrypt("secret")
	b, _ := v.Encrypt("secret")
	if a == b {
		t.Fatalf("nonce should cause ciphertexts to differ")
	}
}

func TestVault_TamperedCiphertext(t *testing.T) {
	v := newTestVault(t, 1)
	ct, _ := v.Encrypt("important")
	tampered := ct[:len(ct)-1] + "A"
	if _, err := v.Decrypt(tampered); err != ErrCiphertext {
		t.Fatalf("expected ErrCiphertext on tampered envelope, got %v", err)
	}
}

func TestVault_UnknownKeyVersion(t *testing.T) {
	v := newTestVault(t, 1)
	ct, _ := v.Encrypt("secret")
	// Rebuild the vault without the key version used for the ciphertext.
	other, _ := NewVault(map[uint16][]byte{2: bytes.Repeat([]byte("x"), 32)})
	if _, err := other.Decrypt(ct); err != ErrKeyUnknown {
		t.Fatalf("expected ErrKeyUnknown, got %v", err)
	}
}

func TestVault_RotationOldKeysStillReadable(t *testing.T) {
	// Build a v=1 vault, encrypt, then build a vault with v=1 and v=2.
	// Reads of the old ciphertext must still succeed, new writes use v=2.
	keysV1 := map[uint16][]byte{1: bytes.Repeat([]byte("a"), 32)}
	v1, _ := NewVault(keysV1)
	oldCT, _ := v1.Encrypt("legacy")

	keysBoth := map[uint16][]byte{
		1: bytes.Repeat([]byte("a"), 32),
		2: bytes.Repeat([]byte("b"), 32),
	}
	v2, _ := NewVault(keysBoth)
	if v2.ActiveVersion() != 2 {
		t.Fatalf("expected active v=2, got %d", v2.ActiveVersion())
	}
	if got, err := v2.Decrypt(oldCT); err != nil || got != "legacy" {
		t.Fatalf("old ciphertext unreadable: %q err=%v", got, err)
	}
	newCT, _ := v2.Encrypt("fresh")
	if newCT == oldCT {
		t.Fatalf("new ciphertext should differ from legacy")
	}
}

func TestNewVault_RejectsBadKeyLen(t *testing.T) {
	if _, err := NewVault(map[uint16][]byte{1: []byte("short")}); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestNewVault_RequiresAtLeastOneKey(t *testing.T) {
	if _, err := NewVault(nil); err != ErrNoActiveKey {
		t.Fatalf("expected ErrNoActiveKey, got %v", err)
	}
}
