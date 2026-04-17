package api

import (
	"encoding/csv"
	"net/http"
	"strconv"

	"github.com/eaglepoint/oops/backend/internal/filter"
	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/eaglepoint/oops/backend/internal/order"
	"github.com/eaglepoint/oops/backend/internal/store"
	"github.com/labstack/echo/v4"
)

// ExportOrdersCSV streams a CSV of orders selected by the supplied
// filter. It is gated by the `orders.export` permission and re-uses
// `filter.Validate` so overly broad exports (size > 100 without a
// narrowing clause) are rejected with the same error path that
// protects saved filters. Every export is audit-logged with the
// final resolved filter payload so reviewers can reconstruct which
// rows an analyst pulled.
func (s *Server) ExportOrdersCSV(c echo.Context) error {
	var body filter.Filter
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	body.Entity = filter.EntityOrder
	if err := body.Validate(order.AllStatuses); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Hard cap at MaxExportSize rows regardless of page/size so a caller
	// who legitimately passes a narrowing clause still can't accidentally
	// dump more than the policy allows in a single request.
	limit := body.Size
	if limit <= 0 || limit > filter.MaxExportSize {
		limit = filter.MaxExportSize
	}

	q := store.OrderQuery{
		Keyword:  body.Keyword,
		Statuses: body.Statuses,
		Tags:     body.Tags,
		Priority: body.Priority,
		SortBy:   body.SortBy,
		SortDesc: body.SortDesc,
		Limit:    limit,
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
		ts := t.Unix() + 86399
		q.EndUnix = &ts
	}
	if body.MinPriceUSD != nil {
		v := int(*body.MinPriceUSD * 100)
		q.MinCents = &v
	}
	if body.MaxPriceUSD != nil {
		v := int(*body.MaxPriceUSD * 100)
		q.MaxCents = &v
	}
	ctx := c.Request().Context()
	rows, total, err := s.Store.QueryOrders(ctx, q)
	if err != nil {
		return httpx.WriteError(c, err)
	}

	sess := httpx.CurrentSession(c)
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c),
		"order", "export", "export", "", nil,
		map[string]any{
			"filter":   body,
			"returned": len(rows),
			"total":    total,
		})

	// Stream the CSV into the response. We write a sentinel header
	// before the first row so the browser treats the response as a
	// download.
	w := c.Response()
	w.Header().Set(echo.HeaderContentType, "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="orders-export.csv"`)
	w.WriteHeader(http.StatusOK)

	cw := csv.NewWriter(w)
	// Column header mirrors the fields a reviewer would inspect to
	// reconstruct the audit trail.
	if err := cw.Write([]string{
		"id", "status", "priority", "total_cents", "customer_id",
		"placed_at", "updated_at", "tags", "delivery_city", "delivery_zip",
	}); err != nil {
		return err
	}
	for _, o := range rows {
		if err := cw.Write(orderCSVRow(o)); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// orderCSVRow renders one order as a stable row for the CSV export.
func orderCSVRow(o order.Order) []string {
	return []string{
		o.ID,
		string(o.Status),
		o.Priority,
		strconv.Itoa(o.TotalCents),
		o.CustomerID,
		o.PlacedAt.UTC().Format("2006-01-02T15:04:05Z"),
		o.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		csvJoin(o.Tags),
		o.DeliveryCity,
		o.DeliveryZIP,
	}
}

// csvJoin compresses a []string into a single CSV cell using `;` as
// the inner separator so Excel/Sheets can re-split if needed without
// colliding with the outer `,` delimiter.
func csvJoin(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ";"
		}
		out += x
	}
	return out
}

