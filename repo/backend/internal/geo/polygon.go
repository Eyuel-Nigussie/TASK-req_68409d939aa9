// Package geo provides offline geospatial helpers: point-in-polygon
// evaluation against locally configured service-area polygons, and a
// road-distance approximation that falls back to great-circle distance
// when no preloaded route table entry is available.
//
// All computations are deterministic and internet-free so that dispatch
// validation continues to work on an isolated local network.
package geo

import (
	"errors"
	"math"
)

// Point represents a WGS-84 coordinate in decimal degrees.
type Point struct {
	Lat float64
	Lng float64
}

// Polygon is an ordered list of vertices. The polygon is implicitly closed;
// callers do not need to repeat the first vertex as the last.
type Polygon struct {
	// ID identifies the polygon (e.g., "ZoneA").
	ID       string
	Vertices []Point
}

// Region groups a polygon with an optional fee table. A location is "in
// service" when it lies inside any region's polygon.
type Region struct {
	Polygon Polygon
	// BaseFeeCents is the minimum fee charged for deliveries into the region,
	// regardless of distance. Kept in cents to avoid float rounding bugs.
	BaseFeeCents int
	// PerMileFeeCents is added per mile of road distance.
	PerMileFeeCents int
}

// ErrNoRegion indicates a point falls outside every configured region.
var ErrNoRegion = errors.New("location is outside configured service area")

// Contains returns true iff p lies strictly inside or on the boundary of
// the polygon. Uses the ray-casting algorithm (O(n)) which is sufficient
// at the scale of a regional service territory (thousands of vertices at
// most). The algorithm is deterministic under identical inputs which is
// required for audit reproducibility.
func (poly Polygon) Contains(p Point) bool {
	n := len(poly.Vertices)
	if n < 3 {
		return false
	}
	inside := false
	j := n - 1
	for i := 0; i < n; i++ {
		vi := poly.Vertices[i]
		vj := poly.Vertices[j]
		// Boundary short-circuit: treat a point that lies exactly on a
		// polygon edge as "inside". Dispatch operators expect a pin that
		// lands on the boundary of the service zone to be accepted.
		if onSegment(vi, vj, p) {
			return true
		}
		if (vi.Lat > p.Lat) != (vj.Lat > p.Lat) {
			x := (vj.Lng-vi.Lng)*(p.Lat-vi.Lat)/(vj.Lat-vi.Lat) + vi.Lng
			if p.Lng < x {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

// onSegment reports whether point p lies on segment a-b within a small
// tolerance. The tolerance is chosen to match ~1m at typical latitudes.
func onSegment(a, b, p Point) bool {
	const eps = 1e-9
	cross := (b.Lat-a.Lat)*(p.Lng-a.Lng) - (b.Lng-a.Lng)*(p.Lat-a.Lat)
	if math.Abs(cross) > eps {
		return false
	}
	dot := (p.Lat-a.Lat)*(b.Lat-a.Lat) + (p.Lng-a.Lng)*(b.Lng-a.Lng)
	if dot < -eps {
		return false
	}
	sqLen := (b.Lat-a.Lat)*(b.Lat-a.Lat) + (b.Lng-a.Lng)*(b.Lng-a.Lng)
	return dot <= sqLen+eps
}

// RegionForPoint returns the first region that contains p, or ErrNoRegion.
// The order of regions is significant because overlapping zones are
// resolved by priority-of-input (the configuration UI sorts them).
func RegionForPoint(regions []Region, p Point) (*Region, error) {
	for i := range regions {
		if regions[i].Polygon.Contains(p) {
			return &regions[i], nil
		}
	}
	return nil, ErrNoRegion
}
