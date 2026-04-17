package geo

import (
	"math"
	"testing"
)

func approxEqual(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestHaversineMiles_KnownPairs(t *testing.T) {
	// NYC (40.7128,-74.0060) to LA (34.0522,-118.2437). Depending on the
	// Earth-radius constant choice (3958.76 statute miles here) the answer
	// comes out ~2446 mi; we tolerate ±15 mi to match the range of common
	// references for this pair.
	nyc := Point{40.7128, -74.0060}
	la := Point{34.0522, -118.2437}
	d := HaversineMiles(nyc, la)
	if !approxEqual(d, 2446, 15) {
		t.Fatalf("NYC->LA expected ~2446 mi (±15), got %.1f", d)
	}
	// Identity is zero.
	if HaversineMiles(nyc, nyc) > 1e-6 {
		t.Fatalf("zero distance required for identical points")
	}
	// Symmetry.
	if HaversineMiles(nyc, la) != HaversineMiles(la, nyc) {
		t.Fatalf("haversine must be symmetric")
	}
}

func TestRouteTable_NormalizesKey(t *testing.T) {
	r := NewRouteTable()
	r.Add("A", "B", 12.5)
	if v, ok := r.Lookup("B", "A"); !ok || v != 12.5 {
		t.Fatalf("reversed lookup failed: %v %v", v, ok)
	}
}

func TestRouteTable_SameWaypointZero(t *testing.T) {
	r := NewRouteTable()
	if v, ok := r.Lookup("X", "X"); !ok || v != 0 {
		t.Fatalf("same waypoint should be 0, got %v %v", v, ok)
	}
}

func TestRouteTable_DistanceUsesTableWhenPresent(t *testing.T) {
	r := NewRouteTable()
	r.Add("A", "B", 7.0)
	res := r.Distance("A", "B", Point{0, 0}, Point{0, 1})
	if res.Method != "route_table" || res.Miles != 7.0 {
		t.Fatalf("expected route table usage, got %+v", res)
	}
}

func TestRouteTable_DistanceFallbackWhenMissing(t *testing.T) {
	r := NewRouteTable()
	nyc := Point{40.7128, -74.0060}
	la := Point{34.0522, -118.2437}
	res := r.Distance("NYC", "LA", nyc, la)
	if res.Method != "haversine" {
		t.Fatalf("expected haversine fallback, got %q", res.Method)
	}
	if !approxEqual(res.Miles, 2451, 10) {
		t.Fatalf("haversine fallback wrong: %v", res.Miles)
	}
}

func TestFeeCents(t *testing.T) {
	region := &Region{BaseFeeCents: 500, PerMileFeeCents: 25}
	cases := []struct {
		miles float64
		want  int
	}{
		{0, 500},
		{1, 525},
		{4, 600},
		{10.5, 763},
	}
	for _, c := range cases {
		if got := FeeCents(region, c.miles); got != c.want {
			t.Errorf("FeeCents(%v) = %d, want %d", c.miles, got, c.want)
		}
	}
	if got := FeeCents(nil, 10); got != 0 {
		t.Errorf("nil region should yield 0, got %d", got)
	}
}
