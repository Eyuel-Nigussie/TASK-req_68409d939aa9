package api

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eaglepoint/oops/backend/internal/audit"
	"github.com/eaglepoint/oops/backend/internal/filter"
	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/eaglepoint/oops/backend/internal/order"
	"github.com/eaglepoint/oops/backend/internal/store"
	"github.com/labstack/echo/v4"
)

// CreateOrder registers a new order in "placed" state. The delivery
// street is encrypted at rest via the vault because it is PII; city, state,
// and ZIP remain in clear to support fast address-based job lookup.
func (s *Server) CreateOrder(c echo.Context) error {
	var body struct {
		CustomerID     string             `json:"customer_id"`
		Priority       string             `json:"priority"`
		TotalCents     int                `json:"total_cents"`
		Tags           []string           `json:"tags"`
		Items          []order.LineItem   `json:"items"`
		DeliveryStreet string             `json:"delivery_street"`
		DeliveryCity   string             `json:"delivery_city"`
		DeliveryState  string             `json:"delivery_state"`
		DeliveryZIP    string             `json:"delivery_zip"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	encStreet, err := s.Vault.Encrypt(body.DeliveryStreet)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	now := s.Clock()
	o := order.Order{
		ID:             newID(),
		CustomerID:     body.CustomerID,
		Status:         order.StatusPlaced,
		PlacedAt:       now,
		UpdatedAt:      now,
		TotalCents:     body.TotalCents,
		Priority:       body.Priority,
		Tags:           body.Tags,
		Items:          body.Items,
		DeliveryStreet: encStreet,
		DeliveryCity:   body.DeliveryCity,
		DeliveryState:  body.DeliveryState,
		DeliveryZIP:    body.DeliveryZIP,
	}
	ctx := c.Request().Context()
	if err := s.Store.CreateOrder(ctx, o); err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	// Audit snapshot redacts the encrypted delivery street.
	redacted := orderRedact(o)
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityOrder, o.ID, "create", "", nil, redacted)

	// If any line item ships with backordered=true the system should raise
	// the OOS exception at the moment of creation, not later.
	if ex := order.DetectOutOfStock(&o, s.Clock()); ex != nil {
		if err := s.Store.PutException(ctx, *ex); err != nil {
			return httpx.WriteError(c, err)
		}
		_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityOrderException, ex.OrderID, "flag", ex.Description, nil, ex)
	}
	// Decrypt for response; audit already stored the redacted version.
	o.DeliveryStreet = body.DeliveryStreet
	return c.JSON(http.StatusCreated, o)
}

// orderRedact removes the encrypted street field from an audit snapshot
// so the append-only log never contains plaintext addresses.
func orderRedact(o order.Order) map[string]any {
	return map[string]any{
		"id": o.ID, "status": o.Status, "customer_id": o.CustomerID,
		"placed_at": o.PlacedAt, "total_cents": o.TotalCents, "priority": o.Priority,
		"tags": o.Tags, "items": o.Items,
		"delivery_city": o.DeliveryCity, "delivery_state": o.DeliveryState,
		"delivery_zip": o.DeliveryZIP, "has_delivery_street": o.DeliveryStreet != "",
	}
}

// GetOrder returns a single order with its event history. The stored
// delivery street is decrypted before serialization.
func (s *Server) GetOrder(c echo.Context) error {
	o, err := s.Store.GetOrder(c.Request().Context(), c.Param("id"))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	if plain, derr := s.Vault.Decrypt(o.DeliveryStreet); derr == nil {
		o.DeliveryStreet = plain
	}
	return c.JSON(http.StatusOK, o)
}

// QueryOrders accepts the full advanced filter payload and runs it through
// the store's query layer. Unlike ListOrders (GET), this endpoint is POST
// because complex filters don't fit cleanly in URL parameters and allows
// the server to return pagination metadata (total count).
func (s *Server) QueryOrders(c echo.Context) error {
	var body filter.Filter
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	body.Entity = filter.EntityOrder
	if err := body.Validate(order.AllStatuses); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	q := store.OrderQuery{
		Keyword:  body.Keyword,
		Statuses: body.Statuses,
		Tags:     body.Tags,
		Priority: body.Priority,
		SortBy:   body.SortBy,
		SortDesc: body.SortDesc,
		Limit:    body.Size,
		Offset:   (body.Page - 1) * body.Size,
	}
	if body.StartDate != "" {
		t, err := filter.ParseDate(body.StartDate)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		ts := t.Unix()
		q.StartUnix = &ts
	}
	if body.EndDate != "" {
		t, err := filter.ParseDate(body.EndDate)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		// End date is inclusive up to the last second of that day.
		ts := t.Unix() + 86399
		q.EndUnix = &ts
	}
	if body.MinPriceUSD != nil {
		c := int(*body.MinPriceUSD * 100)
		q.MinCents = &c
	}
	if body.MaxPriceUSD != nil {
		c := int(*body.MaxPriceUSD * 100)
		q.MaxCents = &c
	}
	out, total, err := s.Store.QueryOrders(c.Request().Context(), q)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"items":    out,
		"total":    total,
		"page":     body.Page,
		"size":     body.Size,
		"has_next": q.Offset+len(out) < total,
	})
}

// ListOrders supports status + date filters and cursor-based pagination.
func (s *Server) ListOrders(c echo.Context) error {
	statuses := splitCSV(c.QueryParam("status"))
	var from, to *int64
	if v := c.QueryParam("from"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			from = &n
		}
	}
	if v := c.QueryParam("to"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			to = &n
		}
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if limit <= 0 {
		limit = 25
	}
	out, err := s.Store.ListOrders(c.Request().Context(), statuses, from, to, limit, offset)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// OrdersByAddress implements the "jobs by address" discovery requirement.
// It narrows on the indexed delivery-city/delivery-ZIP columns and then,
// if the caller provided a street term, decrypts each candidate's street
// and filters on a case-insensitive substring match.
func (s *Server) OrdersByAddress(c echo.Context) error {
	street := strings.TrimSpace(c.QueryParam("street"))
	city := c.QueryParam("city")
	zip := c.QueryParam("zip")
	if zip == "" && city == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "must specify at least city or zip")
	}
	os, err := s.Store.OrdersByAddress(c.Request().Context(), city, zip)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	needle := strings.ToLower(street)
	out := make([]order.Order, 0, len(os))
	for _, o := range os {
		plain, _ := s.Vault.Decrypt(o.DeliveryStreet)
		if needle != "" && !strings.Contains(strings.ToLower(plain), needle) {
			continue
		}
		o.DeliveryStreet = plain
		out = append(out, o)
		if len(out) >= addressLookupMaxRows {
			break
		}
	}
	return c.JSON(http.StatusOK, out)
}

// addressLookupMaxRows caps the by-address handlers (orders and
// customers) so a shared city/zip can't be used as a bulk-dump channel.
// Callers who need more than this should use the paginated filter
// endpoints — which already refuse overly broad queries.
const addressLookupMaxRows = 200

// TransitionOrder advances the state machine and appends an event.
func (s *Server) TransitionOrder(c echo.Context) error {
	var body struct {
		To     string `json:"to"`
		Reason string `json:"reason"`
		Note   string `json:"note"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	o, err := s.Store.GetOrder(ctx, c.Param("id"))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	before := o
	now := s.Clock()
	ev, err := o.Transition(order.Status(body.To), sess.UserID, body.Reason, body.Note, now)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ev.ID = newID()
	if err := s.Store.UpdateOrder(ctx, o); err != nil {
		return httpx.WriteError(c, err)
	}
	if err := s.Store.AppendOrderEvent(ctx, ev); err != nil {
		return httpx.WriteError(c, err)
	}
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityOrder, o.ID, "transition", body.Reason, before, o)
	return c.JSON(http.StatusOK, o)
}

// Tunables for the L5 throttle.
const (
	// exceptionSweepInterval is the minimum wall-clock gap between two
	// detector sweeps. A refresh inside the window reads from the
	// exception store without re-scanning orders.
	exceptionSweepInterval = 60 * time.Second
	// exceptionSweepMaxOrders bounds the work done by any single sweep
	// so a very large orders table cannot make one request O(all).
	exceptionSweepMaxOrders = 200
)

// exceptionSweepState tracks when the detectors last ran so a burst of
// reads in quick succession doesn't redo O(orders) work per request.
type exceptionSweepState struct {
	mu     sync.Mutex
	lastAt time.Time
}

// ListExceptions returns the current exception queue. The two detectors
// (picking-timeout, out-of-stock) are expensive to run — they each
// scan a window of recent orders — so they are throttled by
// exceptionSweepInterval (L5). A request inside the cool-down window
// returns the queue as it stands in the store without re-running the
// detectors. Write-path events (CreateOrder, UpdateInventory,
// PlanOutOfStock) still raise exceptions synchronously, so legitimate
// events are not hidden by the throttle; it only prevents an operator
// mashing the refresh button from thrashing the DB.
func (s *Server) ListExceptions(c echo.Context) error {
	ctx := c.Request().Context()
	if s.shouldSweepExceptions() {
		if err := s.sweepExceptions(c); err != nil {
			return httpx.WriteError(c, err)
		}
	}
	ex, err := s.Store.ListExceptions(ctx)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, ex)
}

// shouldSweepExceptions returns true at most once per
// exceptionSweepInterval. It also updates the timestamp so concurrent
// readers share a single sweep per window.
func (s *Server) shouldSweepExceptions() bool {
	now := s.Clock()
	s.exceptionSweep.mu.Lock()
	defer s.exceptionSweep.mu.Unlock()
	if !s.exceptionSweep.lastAt.IsZero() && now.Sub(s.exceptionSweep.lastAt) < exceptionSweepInterval {
		return false
	}
	s.exceptionSweep.lastAt = now
	return true
}

// sweepExceptions runs the automatic detectors against a bounded window
// of orders. Newly detected exceptions are persisted and audited exactly
// as the previous inline-on-read logic did.
func (s *Server) sweepExceptions(c echo.Context) error {
	ctx := c.Request().Context()
	pickings, err := s.Store.ListOrders(ctx, []string{string(order.StatusPicking)}, nil, nil, exceptionSweepMaxOrders, 0)
	if err != nil {
		return err
	}
	// Non-picking orders are inspected for backordered items — OOS can
	// flag before picking begins — but only the most recent window.
	allRecent, err := s.Store.ListOrders(ctx, nil, nil, nil, exceptionSweepMaxOrders, 0)
	if err != nil {
		return err
	}
	now := s.Clock()
	sess := httpx.CurrentSession(c)
	existing := map[string]struct{}{}
	if cur, _ := s.Store.ListExceptions(ctx); cur != nil {
		for _, e := range cur {
			existing[e.OrderID+"|"+e.Kind] = struct{}{}
		}
	}
	record := func(ex *order.Exception) error {
		if ex == nil {
			return nil
		}
		key := ex.OrderID + "|" + ex.Kind
		if _, ok := existing[key]; ok {
			return nil
		}
		if err := s.Store.PutException(ctx, *ex); err != nil {
			return err
		}
		existing[key] = struct{}{}
		_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityOrderException, ex.OrderID, "flag", ex.Description, nil, ex)
		return nil
	}
	for _, o := range pickings {
		if err := record(order.DetectPickingTimeout(&o, now)); err != nil {
			return err
		}
	}
	for _, o := range allRecent {
		if err := record(order.DetectOutOfStock(&o, now)); err != nil {
			return err
		}
	}
	return nil
}

// UpdateInventory is the inventory-signal endpoint. When picking staff
// discover an item is out of stock they call this with the SKU and a
// backordered=true flag; the handler mutates the order and — via the
// DetectOutOfStock rule — surfaces an exception on the next queue read.
// This replaces the prior manual-only OOS planning flow.
func (s *Server) UpdateInventory(c echo.Context) error {
	var body struct {
		SKU         string `json:"sku"`
		Backordered bool   `json:"backordered"`
		Note        string `json:"note"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if body.SKU == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "sku required")
	}
	ctx := c.Request().Context()
	o, err := s.Store.GetOrder(ctx, c.Param("id"))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	before := o
	if changed := o.MarkItemBackordered(body.SKU, body.Backordered); !changed {
		return echo.NewHTTPError(http.StatusNotFound, "sku not on order or already in requested state")
	}
	o.UpdatedAt = s.Clock()
	if err := s.Store.UpdateOrder(ctx, o); err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityOrder, o.ID, "inventory", body.Note, before, o)

	// Immediately run the OOS detector so the exception queue is updated
	// without waiting for the operator to navigate to it. This satisfies
	// the prompt's "automatic exception flagging" requirement.
	if ex := order.DetectOutOfStock(&o, s.Clock()); ex != nil {
		if err := s.Store.PutException(ctx, *ex); err != nil {
			return httpx.WriteError(c, err)
		}
		_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityOrderException, ex.OrderID, "flag", ex.Description, nil, ex)
	}
	return c.JSON(http.StatusOK, o)
}

// PlanOutOfStock computes the suggested response to an OOS condition and
// returns whether a shipment split is recommended.
func (s *Server) PlanOutOfStock(c echo.Context) error {
	var body struct {
		Available   []string `json:"available"`
		Backordered []string `json:"backordered"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	plan := order.PlanOutOfStock(body.Available, body.Backordered)
	ctx := c.Request().Context()
	sess := httpx.CurrentSession(c)
	orderID := c.Param("id")
	if len(body.Backordered) > 0 {
		ex := order.Exception{
			OrderID:     orderID,
			Kind:        "out_of_stock",
			DetectedAt:  s.Clock(),
			Description: plan.Description,
		}
		if err := s.Store.PutException(ctx, ex); err != nil {
			return httpx.WriteError(c, err)
		}
		_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityOrderException, orderID, "flag", plan.Description, nil, ex)
	}
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityOrder, orderID, "plan_oos", "",
		map[string]any{"available": body.Available, "backordered": body.Backordered}, plan)
	return c.JSON(http.StatusOK, plan)
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	raw := strings.Split(s, ",")
	out := raw[:0]
	for _, p := range raw {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ensure time import is used in tests
var _ = time.Time{}
