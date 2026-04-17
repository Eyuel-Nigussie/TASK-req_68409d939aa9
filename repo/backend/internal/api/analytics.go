package api

import (
	"net/http"
	"strconv"

	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/labstack/echo/v4"
)

// Analytics handlers produce deterministic, bounded aggregates for the
// analyst workspace. All time windows are optional; when absent the store
// aggregates across all rows. The "from" and "to" query params are Unix
// seconds so the client doesn't need timezone logic.

func parseWindow(c echo.Context) (int64, int64) {
	from, _ := strconv.ParseInt(c.QueryParam("from"), 10, 64)
	to, _ := strconv.ParseInt(c.QueryParam("to"), 10, 64)
	return from, to
}

// AnalyticsOrderStatus returns counts of orders by status within the window.
func (s *Server) AnalyticsOrderStatus(c echo.Context) error {
	from, to := parseWindow(c)
	out, err := s.Store.OrderStatusCounts(c.Request().Context(), from, to)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// AnalyticsOrdersPerDay returns a daily time-series of order counts.
func (s *Server) AnalyticsOrdersPerDay(c echo.Context) error {
	from, to := parseWindow(c)
	out, err := s.Store.OrdersPerDay(c.Request().Context(), from, to)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// AnalyticsSampleStatus mirrors the order-status aggregation for samples.
func (s *Server) AnalyticsSampleStatus(c echo.Context) error {
	from, to := parseWindow(c)
	out, err := s.Store.SampleStatusCounts(c.Request().Context(), from, to)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// AnalyticsAbnormalRate is the "quality" KPI: fraction of measurements
// flagged abnormal across reports issued inside the window.
func (s *Server) AnalyticsAbnormalRate(c echo.Context) error {
	from, to := parseWindow(c)
	out, err := s.Store.AbnormalReportRate(c.Request().Context(), from, to)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// AnalyticsExceptionsByKind shows how many of each exception kind are
// currently open (unresolved). Useful for the operations dashboard.
func (s *Server) AnalyticsExceptionsByKind(c echo.Context) error {
	out, err := s.Store.ExceptionCountsByKind(c.Request().Context())
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// AnalyticsSummary rolls the key KPIs into one payload so the Analytics
// dashboard can hydrate with a single request.
func (s *Server) AnalyticsSummary(c echo.Context) error {
	ctx := c.Request().Context()
	from, to := parseWindow(c)
	orderStatus, _ := s.Store.OrderStatusCounts(ctx, from, to)
	sampleStatus, _ := s.Store.SampleStatusCounts(ctx, from, to)
	series, _ := s.Store.OrdersPerDay(ctx, from, to)
	abnormal, _ := s.Store.AbnormalReportRate(ctx, from, to)
	excp, _ := s.Store.ExceptionCountsByKind(ctx)
	return c.JSON(http.StatusOK, map[string]any{
		"order_status":   orderStatus,
		"sample_status":  sampleStatus,
		"orders_per_day": series,
		"abnormal_rate":  abnormal,
		"exceptions":     excp,
	})
}
