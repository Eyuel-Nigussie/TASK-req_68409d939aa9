package geo

import (
	"testing"
)

func TestFeeCents_DefaultRoundingFromPartialCents(t *testing.T) {
	region := &Region{BaseFeeCents: 500, PerMileFeeCents: 10}
	// 0.04 * 10 = 0.4, +epsilon still <0.5 → floor rounds to 0, +base=500.
	if got := FeeCents(region, 0.04); got != 500 {
		t.Fatalf("got %d", got)
	}
	// 0.05 * 10 = 0.5, rounds up to 1, +base = 501.
	if got := FeeCents(region, 0.05); got != 501 {
		t.Fatalf("got %d", got)
	}
}

func TestRouteTable_SelfDistanceBypassesLookup(t *testing.T) {
	r := NewRouteTable()
	d := r.Distance("A", "A", Point{1, 2}, Point{3, 4})
	if d.Miles != 0 || d.Method != "route_table" {
		t.Fatalf("self-distance wrong: %+v", d)
	}
}

func TestRouteKey_Normalization(t *testing.T) {
	k1 := NormalizedRouteKey("b", "a")
	k2 := NormalizedRouteKey("a", "b")
	if k1 != k2 {
		t.Fatalf("normalization mismatch: %+v vs %+v", k1, k2)
	}
}

func TestContains_ZeroVertexPolygon(t *testing.T) {
	p := Polygon{}
	if p.Contains(Point{0, 0}) {
		t.Fatal("empty polygon should not contain any point")
	}
}

func TestRegionForPoint_EmptyInput(t *testing.T) {
	if _, err := RegionForPoint(nil, Point{0, 0}); err != ErrNoRegion {
		t.Fatalf("expected ErrNoRegion, got %v", err)
	}
}
