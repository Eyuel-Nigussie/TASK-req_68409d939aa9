package geo

import "math"

// earthRadiusMiles is the mean Earth radius in statute miles. We use statute
// miles because the product is US-facing and fee tables are priced per mile.
const earthRadiusMiles = 3958.7613

// HaversineMiles returns the great-circle distance between two points in
// statute miles. Accurate to a few meters for spans relevant to a regional
// territory (<100 miles).
func HaversineMiles(a, b Point) float64 {
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLng := (b.Lng - a.Lng) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Asin(math.Min(1, math.Sqrt(h)))
	return earthRadiusMiles * c
}

// RouteKey uniquely identifies an origin/destination pair in the preloaded
// route table. Keys are canonicalized so (a,b) and (b,a) share an entry.
type RouteKey struct {
	A, B string
}

// NormalizedRouteKey returns a canonical key that is independent of
// argument order.
func NormalizedRouteKey(a, b string) RouteKey {
	if a <= b {
		return RouteKey{A: a, B: b}
	}
	return RouteKey{A: b, B: a}
}

// RouteTable is the preloaded origin/destination distance matrix used when
// road network data is available offline. Distances are in statute miles.
type RouteTable struct {
	m map[RouteKey]float64
}

// NewRouteTable builds an empty route table.
func NewRouteTable() *RouteTable {
	return &RouteTable{m: make(map[RouteKey]float64)}
}

// Add records a road distance between two named waypoints. Calling Add with
// either argument order has the same effect.
func (r *RouteTable) Add(a, b string, miles float64) {
	r.m[NormalizedRouteKey(a, b)] = miles
}

// Lookup returns the recorded road distance, or (0, false) when absent.
func (r *RouteTable) Lookup(a, b string) (float64, bool) {
	if a == b {
		return 0, true
	}
	v, ok := r.m[NormalizedRouteKey(a, b)]
	return v, ok
}

// DistanceResult records both the miles and which method produced it, so
// audit records can capture the precise calculation used.
type DistanceResult struct {
	Miles  float64
	Method string // "route_table" or "haversine"
}

// Distance computes the road distance between two named waypoints when a
// route entry exists; otherwise it falls back to straight-line (haversine).
// The fallback is deterministic so repeated calls with the same inputs
// produce identical results, which the audit system relies on.
func (r *RouteTable) Distance(fromID, toID string, from, to Point) DistanceResult {
	if miles, ok := r.Lookup(fromID, toID); ok {
		return DistanceResult{Miles: miles, Method: "route_table"}
	}
	return DistanceResult{Miles: HaversineMiles(from, to), Method: "haversine"}
}

// FeeCents returns the total fee for a delivery at `miles` into `region`.
// Cent-based integer math avoids surprising float rounding. Rounding for
// fractional cents is half-up so customers are never billed less than the
// published rate.
func FeeCents(region *Region, miles float64) int {
	if region == nil {
		return 0
	}
	// Add a tiny epsilon before flooring to stabilize values that are
	// mathematically integers but stored as 99.99999... due to binary
	// float representation (e.g., 4.02 * 25).
	fractional := miles*float64(region.PerMileFeeCents) + 1e-9
	rounded := int(math.Floor(fractional + 0.5))
	return region.BaseFeeCents + rounded
}
