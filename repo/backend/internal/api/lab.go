package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/eaglepoint/oops/backend/internal/audit"
	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/eaglepoint/oops/backend/internal/lab"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/store"
	"github.com/labstack/echo/v4"
)

// refRanges returns the currently loaded reference-range set. The set is
// hydrated from persistent storage by ReloadRefRanges at startup and again
// after admin updates. Falling back to an empty set here ensures concurrent
// calls during a rare nil-window are safe; Match handles empty sets.
func (s *Server) refRanges() *lab.RangeSet {
	if s.ranges == nil {
		return lab.NewRangeSet()
	}
	return s.ranges
}

// CreateSample records a new collected specimen. When the body
// supplies `test_items[]` (code + instructions), a normalized row is
// written to `test_items` for each entry so the persistence layer
// matches the prompt's "samples, test items" terminology. The
// denormalized `samples.test_codes` array is kept in sync for fast
// filter queries.
func (s *Server) CreateSample(c echo.Context) error {
	var body struct {
		OrderID    string   `json:"order_id"`
		CustomerID string   `json:"customer_id"`
		TestCodes  []string `json:"test_codes"`
		Notes      string   `json:"notes"`
		TestItems  []struct {
			TestCode     string `json:"test_code"`
			Instructions string `json:"instructions"`
		} `json:"test_items"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// Merge test_codes[] and test_items[]: when test_items is present,
	// derive the denormalized TestCodes from it; otherwise accept the
	// bare list.
	codes := body.TestCodes
	if len(body.TestItems) > 0 {
		codes = nil
		for _, t := range body.TestItems {
			if t.TestCode != "" {
				codes = append(codes, t.TestCode)
			}
		}
	}
	if len(codes) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "at least one test_code (or test_item) is required")
	}
	now := s.Clock()
	smp := lab.Sample{
		ID:          newID(),
		OrderID:     body.OrderID,
		CustomerID:  body.CustomerID,
		Status:      lab.SampleSampling,
		CollectedAt: now,
		UpdatedAt:   now,
		TestCodes:   codes,
		Notes:       body.Notes,
	}
	ctx := c.Request().Context()
	if err := s.Store.CreateSample(ctx, smp); err != nil {
		return httpx.WriteError(c, err)
	}
	// Persist the normalized test_items rows. If the client sent only
	// TestCodes we synthesize rows with empty instructions so every
	// sample still gets first-class test_item records the UI can render.
	items := make([]models.TestItem, 0, len(codes))
	if len(body.TestItems) > 0 {
		for _, t := range body.TestItems {
			if t.TestCode == "" {
				continue
			}
			items = append(items, models.TestItem{
				ID: newID(), SampleID: smp.ID, TestCode: t.TestCode,
				Instructions: t.Instructions, CreatedAt: now,
			})
		}
	} else {
		for _, code := range codes {
			items = append(items, models.TestItem{
				ID: newID(), SampleID: smp.ID, TestCode: code, CreatedAt: now,
			})
		}
	}
	if err := s.Store.ReplaceTestItems(ctx, smp.ID, items); err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntitySample, smp.ID, "create", "", nil, smp)
	// Return the sample payload under the same top-level shape that
	// already-deployed tests expect (ID, Status, …) while adding the
	// normalized test_items for clients that care.
	resp := map[string]any{
		"ID":          smp.ID,
		"OrderID":     smp.OrderID,
		"CustomerID":  smp.CustomerID,
		"Status":      smp.Status,
		"CollectedAt": smp.CollectedAt,
		"UpdatedAt":   smp.UpdatedAt,
		"TestCodes":   smp.TestCodes,
		"Notes":       smp.Notes,
		"TestItems":   items,
	}
	return c.JSON(http.StatusCreated, resp)
}

// ListTestItems returns the normalized test-item rows for a sample —
// code + instructions + creation timestamp, so the UI can show the
// technician what tests were ordered and any special guidance.
func (s *Server) ListTestItems(c echo.Context) error {
	items, err := s.Store.ListTestItems(c.Request().Context(), c.Param("id"))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, items)
}

// TransitionSample advances a sample through its lifecycle.
func (s *Server) TransitionSample(c echo.Context) error {
	var body struct {
		To string `json:"to"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	smp, err := s.Store.GetSample(ctx, c.Param("id"))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	before := smp
	sess := httpx.CurrentSession(c)
	if _, err := smp.Transition(lab.SampleStatus(body.To), sess.UserID, s.Clock()); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := s.Store.UpdateSample(ctx, smp); err != nil {
		return httpx.WriteError(c, err)
	}
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntitySample, smp.ID, "transition", "", before, smp)
	return c.JSON(http.StatusOK, smp)
}

// GetSample returns a sample by ID.
func (s *Server) GetSample(c echo.Context) error {
	smp, err := s.Store.GetSample(c.Request().Context(), c.Param("id"))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, smp)
}

// ListSamples supports status filtering.
func (s *Server) ListSamples(c echo.Context) error {
	statuses := splitCSV(c.QueryParam("status"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if limit <= 0 {
		limit = 25
	}
	out, err := s.Store.ListSamples(c.Request().Context(), statuses, limit, offset)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// CreateReportDraft creates a v1 issued report for a sample.
//
// Controlled-workflow guards enforced server-side:
//  1. Sample must exist (404).
//  2. Sample must be in `in_testing` or `reported` (409). A report cannot
//     be issued for a sample that hasn't reached testing.
//  3. Only one v1 report per sample: a second create on an already-reported
//     sample returns 409; callers should use the correction endpoint.
//
// When the sample is in `in_testing`, the handler atomically advances it
// to `reported` so the lifecycle remains consistent with the report it
// just produced.
func (s *Server) CreateReportDraft(c echo.Context) error {
	var body struct {
		Title        string             `json:"title"`
		Narrative    string             `json:"narrative"`
		Measurements []lab.Measurement  `json:"measurements"`
		Demographic  string             `json:"demographic"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if body.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title required")
	}
	ctx := c.Request().Context()
	sampleID := c.Param("id")

	// Gate 1: sample must exist.
	smp, err := s.Store.GetSample(ctx, sampleID)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	// Gate 2: sample must be ready for a report.
	if smp.Status != lab.SampleInTesting && smp.Status != lab.SampleReported {
		return echo.NewHTTPError(http.StatusConflict,
			"sample is not ready for reporting (status="+string(smp.Status)+"); advance to in_testing first")
	}
	// Gate 3: only one v1 per sample. If there's already a report, require
	// the caller to use the correction endpoint.
	if _, err := s.Store.LatestReportForSample(ctx, sampleID); err == nil {
		return echo.NewHTTPError(http.StatusConflict, "sample already has a report; use /api/reports/:id/correct")
	} else if !errors.Is(err, store.ErrNotFound) {
		return httpx.WriteError(c, err)
	}

	ms := lab.EvaluateAll(body.Measurements, s.refRanges(), body.Demographic)
	now := s.Clock()
	sess := httpx.CurrentSession(c)
	r := lab.Report{
		ID:           newID(),
		SampleID:     sampleID,
		Version:      1,
		Status:       lab.ReportIssued,
		Title:        body.Title,
		Narrative:    body.Narrative,
		Measurements: ms,
		AuthorID:     sess.UserID,
		IssuedAt:     now,
		SearchText:   lab.BuildSearchText(body.Title, body.Narrative, ms),
	}
	if err := s.Store.CreateReport(ctx, r); err != nil {
		return httpx.WriteError(c, err)
	}
	// If the sample was in_testing, advance it to reported to keep the
	// lifecycle in sync with the freshly-issued report.
	if smp.Status == lab.SampleInTesting {
		if _, terr := smp.Transition(lab.SampleReported, sess.UserID, now); terr == nil {
			_ = s.Store.UpdateSample(ctx, smp)
			_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntitySample, smp.ID, "transition", "auto-on-report", nil, smp)
		}
	}
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityReport, r.ID, "create", "", nil, r)
	return c.JSON(http.StatusCreated, r)
}

// CorrectReport issues a corrected version that supersedes `id`. The caller
// must pass the expected version for optimistic concurrency.
func (s *Server) CorrectReport(c echo.Context) error {
	var body struct {
		ExpectedVersion int                `json:"expected_version"`
		Title           string             `json:"title"`
		Narrative       string             `json:"narrative"`
		Measurements    []lab.Measurement  `json:"measurements"`
		Demographic     string             `json:"demographic"`
		Reason          string             `json:"reason"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	prior, err := s.Store.GetReport(ctx, c.Param("id"))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	ms := lab.EvaluateAll(body.Measurements, s.refRanges(), body.Demographic)
	sess := httpx.CurrentSession(c)
	next, err := lab.Correct(&prior, body.ExpectedVersion, body.Title, body.Narrative, ms, sess.UserID, body.Reason, s.Clock())
	if err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}
	next.ID = newID()
	prior.SupersededByID = next.ID
	if err := s.Store.ReplaceWithCorrection(ctx, prior, next); err != nil {
		return httpx.WriteError(c, err)
	}
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityReport, next.ID, "correct", body.Reason, prior, next)
	return c.JSON(http.StatusCreated, next)
}

// ListReports returns a page of reports newest-first.
func (s *Server) ListReports(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if limit <= 0 {
		limit = 25
	}
	out, err := s.Store.ListReports(c.Request().Context(), limit, offset)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// SearchReports runs a full-text search across titles and narratives.
func (s *Server) SearchReports(c echo.Context) error {
	q := c.QueryParam("q")
	if q == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "q required")
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 25
	}
	out, err := s.Store.SearchReports(c.Request().Context(), q, limit)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// ArchiveReport marks an issued or superseded report as archived so it
// stops appearing in default lists while remaining retrievable via the
// archive listing and full-text search. Archive is one-way; the reason
// note is stored to satisfy the "every correction requires a reason"
// discipline extended to archival.
func (s *Server) ArchiveReport(c echo.Context) error {
	var body struct {
		Note string `json:"note"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(body.Note) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "archive note is required")
	}
	ctx := c.Request().Context()
	r, err := s.Store.GetReport(ctx, c.Param("id"))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	before := r
	if err := lab.Archive(&r, sess.UserID, body.Note, s.Clock()); err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}
	if err := s.Store.UpdateReport(ctx, r); err != nil {
		return httpx.WriteError(c, err)
	}
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityReport, r.ID, "archive", body.Note, before, r)
	return c.JSON(http.StatusOK, r)
}

// ListArchivedReports returns only archived reports. This is the
// operator-facing view for retention or retrieval flows.
func (s *Server) ListArchivedReports(c echo.Context) error {
	all, err := s.Store.ListArchivedReports(c.Request().Context())
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, all)
}

// GetReport returns a single report by ID.
func (s *Server) GetReport(c echo.Context) error {
	r, err := s.Store.GetReport(c.Request().Context(), c.Param("id"))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, r)
}
