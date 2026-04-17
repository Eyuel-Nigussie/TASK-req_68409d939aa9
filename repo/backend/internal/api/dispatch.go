package api

import (
	"net/http"

	"github.com/eaglepoint/oops/backend/internal/geo"
	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/labstack/echo/v4"
)

// ValidatePin checks whether a given lat/lng lies inside any configured
// service region. The response includes the matched region's ID when
// applicable so the UI can color the pin immediately.
func (s *Server) ValidatePin(c echo.Context) error {
	var body struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	regions, err := s.Store.ListRegions(c.Request().Context())
	if err != nil {
		return httpx.WriteError(c, err)
	}
	p := geo.Point{Lat: body.Lat, Lng: body.Lng}
	region, err := geo.RegionForPoint(regions, p)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"valid":  false,
			"reason": "location is outside configured service area",
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"valid":     true,
		"region_id": region.Polygon.ID,
	})
}

// QuoteFee returns the delivery fee between two named waypoints using the
// route table, falling back to great-circle distance. The response records
// which method was used so the audit log can capture the deterministic
// computation.
func (s *Server) QuoteFee(c echo.Context) error {
	var body struct {
		FromID string  `json:"from_id"`
		ToID   string  `json:"to_id"`
		From   geo.Point `json:"from"`
		To     geo.Point `json:"to"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	regions, err := s.Store.ListRegions(c.Request().Context())
	if err != nil {
		return httpx.WriteError(c, err)
	}
	region, err := geo.RegionForPoint(regions, body.To)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "destination outside service area")
	}
	dist := s.RouteTable.Distance(body.FromID, body.ToID, body.From, body.To)
	fee := geo.FeeCents(region, dist.Miles)
	return c.JSON(http.StatusOK, map[string]any{
		"region_id":   region.Polygon.ID,
		"miles":       dist.Miles,
		"method":      dist.Method,
		"fee_cents":   fee,
		"fee_usd":     float64(fee) / 100.0,
	})
}

// ListRegions is a debugging / admin read-only endpoint.
func (s *Server) ListRegions(c echo.Context) error {
	r, err := s.Store.ListRegions(c.Request().Context())
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, r)
}
