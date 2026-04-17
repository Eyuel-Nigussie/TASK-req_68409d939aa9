package geo

import "testing"

var squarePoly = Polygon{
	ID: "square",
	Vertices: []Point{
		{0, 0}, {0, 10}, {10, 10}, {10, 0},
	},
}

func TestPolygon_ContainsInterior(t *testing.T) {
	if !squarePoly.Contains(Point{5, 5}) {
		t.Fatal("center should be inside")
	}
}

func TestPolygon_ContainsOutside(t *testing.T) {
	for _, p := range []Point{{-1, 5}, {11, 5}, {5, -1}, {5, 11}} {
		if squarePoly.Contains(p) {
			t.Fatalf("expected outside: %v", p)
		}
	}
}

func TestPolygon_ContainsOnBoundary(t *testing.T) {
	// Corners and mid-edges all count as inside.
	for _, p := range []Point{{0, 0}, {0, 5}, {5, 10}, {10, 5}, {5, 0}} {
		if !squarePoly.Contains(p) {
			t.Fatalf("boundary point should be inside: %v", p)
		}
	}
}

func TestPolygon_DegenerateReturnsFalse(t *testing.T) {
	tri := Polygon{Vertices: []Point{{0, 0}, {1, 1}}}
	if tri.Contains(Point{0.5, 0.5}) {
		t.Fatal("fewer than 3 vertices should return false")
	}
}

func TestPolygon_ConcaveShape(t *testing.T) {
	// U-shaped polygon: the notch between (3..7, 0..7) is outside.
	u := Polygon{
		Vertices: []Point{
			{0, 0}, {10, 0}, {10, 10}, {7, 10}, {7, 3},
			{3, 3}, {3, 10}, {0, 10},
		},
	}
	if u.Contains(Point{5, 5}) {
		t.Fatal("notch interior should be outside")
	}
	if !u.Contains(Point{1, 5}) {
		t.Fatal("left arm should be inside")
	}
}

func TestRegionForPoint(t *testing.T) {
	regions := []Region{
		{Polygon: squarePoly, BaseFeeCents: 500, PerMileFeeCents: 25},
	}
	r, err := RegionForPoint(regions, Point{5, 5})
	if err != nil || r.BaseFeeCents != 500 {
		t.Fatalf("expected square region: %v err=%v", r, err)
	}
	if _, err := RegionForPoint(regions, Point{-1, -1}); err != ErrNoRegion {
		t.Fatalf("expected ErrNoRegion, got %v", err)
	}
}

func TestRegionForPoint_OverlapFirstWins(t *testing.T) {
	a := Polygon{ID: "A", Vertices: []Point{{0, 0}, {0, 10}, {10, 10}, {10, 0}}}
	b := Polygon{ID: "B", Vertices: []Point{{0, 0}, {0, 20}, {20, 20}, {20, 0}}}
	regions := []Region{
		{Polygon: a, BaseFeeCents: 100},
		{Polygon: b, BaseFeeCents: 200},
	}
	got, err := RegionForPoint(regions, Point{5, 5})
	if err != nil {
		t.Fatal(err)
	}
	if got.Polygon.ID != "A" {
		t.Fatalf("expected A to win by order, got %s", got.Polygon.ID)
	}
}
