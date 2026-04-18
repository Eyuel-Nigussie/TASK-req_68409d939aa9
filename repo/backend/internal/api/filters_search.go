package api

import (
	"encoding/json"
	"net/http"

	"github.com/eaglepoint/oops/backend/internal/audit"
	"github.com/eaglepoint/oops/backend/internal/filter"
	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/order"
	"github.com/eaglepoint/oops/backend/internal/search"
	"github.com/labstack/echo/v4"
)

// CreateSavedFilter validates and stores a filter in the caller's library.
func (s *Server) CreateSavedFilter(c echo.Context) error {
	var body struct {
		Name   string         `json:"name"`
		Filter filter.Filter  `json:"filter"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if body.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name required")
	}
	knownStatuses := knownStatusesForEntity(body.Filter.Entity)
	if err := body.Filter.Validate(knownStatuses); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// Saved filters are materialized exports-in-waiting: an analyst who
	// later runs this filter can paginate it without further narrowing.
	// Refuse to persist a filter that selects everything, regardless of
	// the size chosen today.
	if !body.Filter.HasNarrowingCriterion() {
		return echo.NewHTTPError(http.StatusBadRequest, filter.ErrTooBroad.Error())
	}
	payload, err := json.Marshal(body.Filter)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	f := models.SavedFilter{
		ID:        newID(),
		OwnerID:   sess.UserID,
		Name:      body.Name,
		Payload:   payload,
		Key:       body.Filter.CanonicalKey(),
		CreatedAt: s.Clock(),
	}
	ctx := c.Request().Context()
	if err := s.Store.CreateSavedFilter(ctx, f); err != nil {
		return httpx.WriteError(c, err)
	}
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntitySavedFilter, f.ID, "create", "",
		nil, map[string]any{"name": f.Name, "key": f.Key})
	return c.JSON(http.StatusCreated, f)
}

// ListSavedFilters returns the caller's filter library.
func (s *Server) ListSavedFilters(c echo.Context) error {
	sess := httpx.CurrentSession(c)
	out, err := s.Store.ListSavedFilters(c.Request().Context(), sess.UserID)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// DeleteSavedFilter removes an owned filter.
func (s *Server) DeleteSavedFilter(c echo.Context) error {
	sess := httpx.CurrentSession(c)
	ctx := c.Request().Context()
	id := c.Param("id")
	if err := s.Store.DeleteSavedFilter(ctx, sess.UserID, id); err != nil {
		return httpx.WriteError(c, err)
	}
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntitySavedFilter, id, "delete", "", nil, nil)
	return c.NoContent(http.StatusNoContent)
}

// GlobalSearch returns a small mixed-type suggestion list that the search
// bar renders in its dropdown. Customers, orders, and reports are all
// searched. Results are pre-filtered and ranked by fuzzy score so UI
// keystrokes with typos still surface relevant records.
func (s *Server) GlobalSearch(c echo.Context) error {
	q := c.QueryParam("q")
	if q == "" {
		return c.JSON(http.StatusOK, []any{})
	}
	ctx := c.Request().Context()
	cs, _ := s.Store.SearchCustomers(ctx, q, 5)
	reports, _ := s.Store.SearchReports(ctx, q, 5)

	// Orders don't have a dedicated full-text column yet, so we pull a
	// bounded recent window and let the fuzzy ranker score locally. The
	// window is small enough (200) for this to remain cheap.
	recent, _ := s.Store.ListOrders(ctx, nil, nil, nil, 200, 0)

	sugs := make([]search.Suggestion, 0, len(cs)+len(reports)+len(recent))
	for _, cu := range cs {
		sugs = append(sugs, search.Suggestion{ID: cu.ID, Label: cu.Name, Kind: "customer"})
	}
	for _, r := range reports {
		sugs = append(sugs, search.Suggestion{ID: r.ID, Label: r.Title, Kind: "report"})
	}
	for _, o := range recent {
		label := "#" + o.ID + " " + string(o.Status) + " " + o.Priority
		if len(o.Tags) > 0 {
			label += " " + joinTags(o.Tags)
		}
		sugs = append(sugs, search.Suggestion{ID: o.ID, Label: label, Kind: "order"})
	}
	ranked := search.Rank(q, sugs, 0.15, 10)
	return c.JSON(http.StatusOK, ranked)
}

func joinTags(tags []string) string {
	out := ""
	for i, t := range tags {
		if i > 0 {
			out += ","
		}
		out += t
	}
	return out
}

// knownStatusesForEntity returns the list of valid status values for the
// given entity, used by filter validation.
func knownStatusesForEntity(entity string) []string {
	switch entity {
	case filter.EntityOrder:
		return order.AllStatuses
	case filter.EntitySample:
		return []string{"sampling", "received", "in_testing", "reported", "rejected"}
	case filter.EntityReport:
		return []string{"draft", "issued", "superseded"}
	default:
		return nil
	}
}
