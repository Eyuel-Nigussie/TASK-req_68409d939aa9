package filter

import (
	"strings"
	"testing"
)

// SortBy on a filter whose Entity isn't known yields ErrUnknownEntity via
// the allowedSort lookup.
func TestFilter_SortOnInvalidEntityNotReached(t *testing.T) {
	// Unknown entities are caught before sort validation, but we still
	// exercise the defensive branch.
	f := &Filter{Entity: "bogus", SortBy: "placed_at"}
	if err := f.Validate(nil); err == nil {
		t.Fatal("expected error")
	}
}

// CanonicalKey: omitting optional fields still produces a stable key.
func TestCanonicalKey_MinimalFilter(t *testing.T) {
	f := &Filter{Entity: EntityCustomer}
	k := f.CanonicalKey()
	if !strings.HasPrefix(k, "customer|") {
		t.Fatalf("unexpected key: %s", k)
	}
}

// Validate with only negative max price and no min price.
func TestFilter_NegativeMaxOnly(t *testing.T) {
	v := -1.0
	f := &Filter{Entity: EntityOrder, MaxPriceUSD: &v}
	if err := f.Validate(nil); err != ErrBadPriceRange {
		t.Fatalf("expected ErrBadPriceRange, got %v", err)
	}
}
