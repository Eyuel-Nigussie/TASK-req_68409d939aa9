package filter

import (
	"testing"
)

func ptrF(v float64) *float64 { return &v }

func TestParseDate(t *testing.T) {
	if _, err := ParseDate(""); err != nil {
		t.Errorf("empty date should be allowed: %v", err)
	}
	if _, err := ParseDate("12/31/2024"); err != nil {
		t.Errorf("valid date should parse: %v", err)
	}
	if _, err := ParseDate("2024-12-31"); err != ErrBadDate {
		t.Errorf("ISO date should be rejected: %v", err)
	}
	if _, err := ParseDate("31/12/2024"); err != ErrBadDate {
		t.Errorf("EU date should be rejected: %v", err)
	}
}

func TestFilter_ValidateHappyPath(t *testing.T) {
	f := &Filter{
		Entity:   EntityOrder,
		Keyword:  "widget",
		Statuses: []string{"placed"},
		StartDate: "01/01/2024",
		EndDate:   "12/31/2024",
		SortBy:   "placed_at",
	}
	if err := f.Validate([]string{"placed", "picking"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Page != 1 || f.Size != DefaultSize {
		t.Fatalf("defaults not applied: %+v", f)
	}
}

func TestFilter_RejectsUnknownEntity(t *testing.T) {
	f := &Filter{Entity: "bogus"}
	if err := f.Validate(nil); err == nil {
		t.Fatal("unknown entity should fail")
	}
}

func TestFilter_RejectsUnknownSort(t *testing.T) {
	f := &Filter{Entity: EntityOrder, SortBy: "drop_table"}
	if err := f.Validate(nil); err == nil {
		t.Fatal("unknown sort should fail")
	}
}

func TestFilter_RejectsUnknownStatus(t *testing.T) {
	f := &Filter{Entity: EntityOrder, Statuses: []string{"warp"}}
	if err := f.Validate([]string{"placed"}); err == nil {
		t.Fatal("unknown status should fail")
	}
}

func TestFilter_RejectsDateOrderInversion(t *testing.T) {
	f := &Filter{Entity: EntityOrder, StartDate: "12/31/2024", EndDate: "01/01/2024"}
	if err := f.Validate(nil); err != ErrDateOrder {
		t.Fatalf("expected ErrDateOrder, got %v", err)
	}
}

func TestFilter_RejectsNegativePrice(t *testing.T) {
	f := &Filter{Entity: EntityOrder, MinPriceUSD: ptrF(-1)}
	if err := f.Validate(nil); err != ErrBadPriceRange {
		t.Fatalf("expected ErrBadPriceRange, got %v", err)
	}
}

func TestFilter_RejectsInvertedPriceRange(t *testing.T) {
	f := &Filter{Entity: EntityOrder, MinPriceUSD: ptrF(100), MaxPriceUSD: ptrF(50)}
	if err := f.Validate(nil); err != ErrBadPriceRange {
		t.Fatalf("expected ErrBadPriceRange, got %v", err)
	}
}

func TestFilter_RejectsTooBroadExport(t *testing.T) {
	f := &Filter{Entity: EntityOrder, Size: 200}
	if err := f.Validate(nil); err != ErrTooBroad {
		t.Fatalf("expected ErrTooBroad, got %v", err)
	}
}

func TestFilter_LargeSizeAllowedWithKeyword(t *testing.T) {
	f := &Filter{Entity: EntityOrder, Size: 200, Keyword: "widget"}
	if err := f.Validate(nil); err != nil {
		t.Fatalf("narrowed export should pass: %v", err)
	}
}

func TestFilter_RejectsPageOutOfRange(t *testing.T) {
	f := &Filter{Entity: EntityOrder, Page: -1, Size: 1}
	if err := f.Validate(nil); err != ErrBadPage {
		t.Fatalf("expected ErrBadPage, got %v", err)
	}
	g := &Filter{Entity: EntityOrder, Page: 1, Size: 1000}
	if err := g.Validate(nil); err != ErrBadPage {
		t.Fatalf("expected ErrBadPage for size=1000, got %v", err)
	}
}

func TestFilter_CanonicalKeyStable(t *testing.T) {
	a := &Filter{Entity: EntityOrder, Statuses: []string{"placed", "picking"}, Tags: []string{"b", "a"}}
	b := &Filter{Entity: EntityOrder, Statuses: []string{"picking", "placed"}, Tags: []string{"a", "b"}}
	if a.CanonicalKey() != b.CanonicalKey() {
		t.Fatalf("order should not matter in canonical key:\n%s\n%s", a.CanonicalKey(), b.CanonicalKey())
	}
}
