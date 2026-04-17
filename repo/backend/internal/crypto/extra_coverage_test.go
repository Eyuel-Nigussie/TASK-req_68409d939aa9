package crypto

import (
	"encoding/base64"
	"testing"
)

func TestDecrypt_RejectsBadPrefix(t *testing.T) {
	v := newTestVault(t, 1)
	if _, err := v.Decrypt("v2:whatever"); err != ErrCiphertext {
		t.Fatalf("expected ErrCiphertext for wrong prefix, got %v", err)
	}
}

func TestDecrypt_RejectsMalformedBase64(t *testing.T) {
	v := newTestVault(t, 1)
	if _, err := v.Decrypt("v1:!!!not-base64!!!"); err != ErrCiphertext {
		t.Fatalf("expected ErrCiphertext for bad base64, got %v", err)
	}
}

func TestDecrypt_RejectsShortBlob(t *testing.T) {
	v := newTestVault(t, 1)
	// Short payload, can't possibly contain nonce+ciphertext.
	short := base64.RawStdEncoding.EncodeToString([]byte{0, 1, 2})
	if _, err := v.Decrypt("v1:" + short); err != ErrCiphertext {
		t.Fatalf("expected ErrCiphertext, got %v", err)
	}
}

func TestDeriveKey_PadsTo32(t *testing.T) {
	k := DeriveKey([]byte("short"))
	if len(k) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(k))
	}
	// Trailing bytes must be zero-pad.
	for _, b := range k[5:] {
		if b != 0 {
			t.Fatalf("expected zero pad, got %v", k)
		}
	}
}

func TestActiveVersion_ReportsHighest(t *testing.T) {
	v := newTestVault(t, 3, 7, 1)
	if v.ActiveVersion() != 7 {
		t.Fatalf("expected active=7, got %d", v.ActiveVersion())
	}
}
