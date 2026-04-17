package api

import (
	"net/http"

	"github.com/eaglepoint/oops/backend/internal/auth"
	"github.com/eaglepoint/oops/backend/internal/geo"
	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/eaglepoint/oops/backend/internal/lab"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/store"
	"github.com/labstack/echo/v4"
)

// AdminCreateUser provisions a new user account. Username must be unique;
// password must satisfy the policy.
func (s *Server) AdminCreateUser(c echo.Context) error {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if body.Username == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username required")
	}
	role := models.Role(body.Role)
	valid := false
	for _, r := range models.AllRoles {
		if r == role {
			valid = true
			break
		}
	}
	if !valid {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid role")
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	now := s.Clock()
	u := models.User{
		ID: newID(), Username: body.Username, Role: role, PasswordHash: hash,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Store.CreateUser(c.Request().Context(), u); err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	_ = s.Audit.Log(c.Request().Context(), sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), "user", u.ID, "create", "",
		nil, map[string]any{"username": u.Username, "role": u.Role})
	return c.JSON(http.StatusCreated, map[string]any{
		"id": u.ID, "username": u.Username, "role": u.Role,
	})
}

// AdminListUsers returns a shallow view of all accounts including their
// persisted lockout state so administrators can see who is currently
// locked and why.
func (s *Server) AdminListUsers(c echo.Context) error {
	ctx := c.Request().Context()
	us, err := s.Store.ListUsers(ctx)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	out := make([]map[string]any, len(us))
	for i, u := range us {
		row := map[string]any{
			"id":         u.ID,
			"username":   u.Username,
			"role":       u.Role,
			"disabled":   u.Disabled,
			"created_at": u.CreatedAt,
		}
		if att, err := s.Store.GetLoginAttempt(ctx, u.Username); err == nil {
			row["failures"] = att.Failures
			row["lock_until"] = att.LockedUntil
		} else {
			row["failures"] = 0
			row["lock_until"] = nil
		}
		out[i] = row
	}
	return c.JSON(http.StatusOK, out)
}

// AdminPutServiceRegions replaces the set of service-area polygons
// atomically. Bodies are geographic JSON with vertex pairs.
func (s *Server) AdminPutServiceRegions(c echo.Context) error {
	var body struct {
		Regions []struct {
			ID              string        `json:"id"`
			Vertices        [][]float64   `json:"vertices"`
			BaseFeeCents    int           `json:"base_fee_cents"`
			PerMileFeeCents int           `json:"per_mile_fee_cents"`
		} `json:"regions"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	regions := make([]geo.Region, 0, len(body.Regions))
	for _, r := range body.Regions {
		if r.ID == "" || len(r.Vertices) < 3 {
			return echo.NewHTTPError(http.StatusBadRequest, "each region needs id and >=3 vertices")
		}
		poly := geo.Polygon{ID: r.ID, Vertices: make([]geo.Point, len(r.Vertices))}
		for i, v := range r.Vertices {
			if len(v) != 2 {
				return echo.NewHTTPError(http.StatusBadRequest, "vertex must be [lat,lng]")
			}
			poly.Vertices[i] = geo.Point{Lat: v[0], Lng: v[1]}
		}
		regions = append(regions, geo.Region{
			Polygon:         poly,
			BaseFeeCents:    r.BaseFeeCents,
			PerMileFeeCents: r.PerMileFeeCents,
		})
	}
	if err := s.Store.ReplaceRegions(c.Request().Context(), regions); err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	_ = s.Audit.Log(c.Request().Context(), sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), "service_regions", "all", "replace", "", nil, body.Regions)
	return c.JSON(http.StatusOK, map[string]any{"count": len(regions)})
}

// AdminAudit returns audit entries filtered by entity.
func (s *Server) AdminAudit(c echo.Context) error {
	entity := c.QueryParam("entity")
	entityID := c.QueryParam("entity_id")
	out, err := s.Audit.List(c.Request().Context(), entity, entityID, 500)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// AdminUpdateUser changes role, password, or disabled flag on a user. The
// caller must be an administrator; all field changes are audited.
func (s *Server) AdminUpdateUser(c echo.Context) error {
	var body struct {
		Role     string `json:"role"`
		Password string `json:"password"`
		Disabled *bool  `json:"disabled"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	u, err := s.Store.GetUserByID(ctx, c.Param("id"))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	before := map[string]any{"role": u.Role, "disabled": u.Disabled}
	if body.Role != "" {
		valid := false
		for _, r := range models.AllRoles {
			if string(r) == body.Role {
				valid = true
				break
			}
		}
		if !valid {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid role")
		}
		u.Role = models.Role(body.Role)
	}
	if body.Password != "" {
		h, err := auth.HashPassword(body.Password)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		u.PasswordHash = h
	}
	if body.Disabled != nil {
		u.Disabled = *body.Disabled
	}
	u.UpdatedAt = s.Clock()
	if err := s.Store.UpdateUser(ctx, u); err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), "user", u.ID, "update", "",
		before, map[string]any{"role": u.Role, "disabled": u.Disabled})
	return c.JSON(http.StatusOK, map[string]any{"id": u.ID, "role": u.Role, "disabled": u.Disabled})
}

// AdminListRefRanges returns the persisted reference-range dictionary.
func (s *Server) AdminListRefRanges(c echo.Context) error {
	rr, err := s.Store.ListRefRanges(c.Request().Context())
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, rr)
}

// AdminPutRefRanges replaces the reference-range dictionary atomically
// and reloads the in-process RangeSet so subsequent report evaluations
// see the new configuration immediately.
func (s *Server) AdminPutRefRanges(c echo.Context) error {
	var body struct {
		Ranges []lab.RefRange `json:"ranges"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// Structural validation: test_code is required; bounds must be ordered
	// when both are present.
	for i, r := range body.Ranges {
		if r.TestCode == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "test_code required (row "+itoa(i)+")")
		}
		if r.LowNormal != nil && r.HighNormal != nil && *r.LowNormal > *r.HighNormal {
			return echo.NewHTTPError(http.StatusBadRequest, "low_normal must be <= high_normal")
		}
		if r.LowCritical != nil && r.HighCritical != nil && *r.LowCritical > *r.HighCritical {
			return echo.NewHTTPError(http.StatusBadRequest, "low_critical must be <= high_critical")
		}
	}
	ctx := c.Request().Context()
	before, _ := s.Store.ListRefRanges(ctx)
	if err := s.Store.ReplaceRefRanges(ctx, body.Ranges); err != nil {
		return httpx.WriteError(c, err)
	}
	if err := s.ReloadRefRanges(ctx); err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), "reference_ranges", "all", "replace", "", before, body.Ranges)
	return c.JSON(http.StatusOK, map[string]any{"count": len(body.Ranges)})
}

// AdminListRoutes returns the persisted road-distance matrix.
func (s *Server) AdminListRoutes(c echo.Context) error {
	rows, err := s.Store.ListRoutes(c.Request().Context())
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, rows)
}

// AdminPutRoutes replaces the route-table contents atomically and reloads
// the in-process RouteTable so dispatch fee quotes start using the new
// distances immediately. Distances are validated to be non-negative.
func (s *Server) AdminPutRoutes(c echo.Context) error {
	var body struct {
		Routes []store.RouteRow `json:"routes"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	for _, r := range body.Routes {
		if r.FromID == "" || r.ToID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "from_id and to_id required")
		}
		if r.Miles < 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "miles must be >= 0")
		}
	}
	ctx := c.Request().Context()
	before, _ := s.Store.ListRoutes(ctx)
	if err := s.Store.ReplaceRoutes(ctx, body.Routes); err != nil {
		return httpx.WriteError(c, err)
	}
	if err := s.ReloadRouteTable(ctx); err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), "route_table", "all", "replace", "", before, body.Routes)
	return c.JSON(http.StatusOK, map[string]any{"count": len(body.Routes)})
}

// itoa is a small local helper so we don't import strconv for a single use.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
