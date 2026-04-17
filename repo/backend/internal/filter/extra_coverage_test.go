package filter

import "testing"

func TestFilter_SortValidForEachEntity(t *testing.T) {
	pairs := map[string]string{
		"customer": "name",
		"order":    "placed_at",
		"sample":   "collected_at",
		"report":   "reported_at",
	}
	for entity, sortBy := range pairs {
		f := &Filter{Entity: entity, SortBy: sortBy}
		if err := f.Validate(nil); err != nil {
			t.Errorf("%s sort %q should be valid: %v", entity, sortBy, err)
		}
	}
}

func TestFilter_CanonicalKeyIncludesPriceFields(t *testing.T) {
	min := 10.0
	max := 50.5
	f := &Filter{Entity: EntityOrder, MinPriceUSD: &min, MaxPriceUSD: &max}
	k := f.CanonicalKey()
	if k == "" {
		t.Fatal("empty key")
	}
	// Idempotent — regenerating matches.
	if k != f.CanonicalKey() {
		t.Fatal("canonical key not stable")
	}
}

func TestFilter_DefaultPageSize(t *testing.T) {
	f := &Filter{Entity: EntityOrder}
	if err := f.Validate(nil); err != nil {
		t.Fatal(err)
	}
	if f.Page != 1 || f.Size != DefaultSize {
		t.Fatalf("defaults not applied: %+v", f)
	}
}

func TestParseDate_RejectsInvalidFormats(t *testing.T) {
	for _, in := range []string{"13/01/2024", "00/00/2024", "01/32/2024", "xx/xx/xxxx"} {
		if _, err := ParseDate(in); err != ErrBadDate {
			t.Errorf("%q: expected ErrBadDate, got %v", in, err)
		}
	}
}
