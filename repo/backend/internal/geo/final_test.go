package geo

import "testing"

// On-segment coverage: a point perpendicular to the segment line falls
// outside (dot < 0 or > sqLen).
func TestOnSegment_OutsideEndpoints(t *testing.T) {
	a := Point{0, 0}
	b := Point{10, 0}
	// Midpoint: dot > 0, sqLen > dot — on segment.
	if !onSegment(a, b, Point{5, 0}) {
		t.Fatal("midpoint must be on segment")
	}
	// Past end of segment.
	if onSegment(a, b, Point{20, 0}) {
		t.Fatal("point past endpoint must not be on segment")
	}
	// Before start.
	if onSegment(a, b, Point{-5, 0}) {
		t.Fatal("point before start must not be on segment")
	}
	// Off the line entirely.
	if onSegment(a, b, Point{5, 1}) {
		t.Fatal("off-line point must not be on segment")
	}
}
