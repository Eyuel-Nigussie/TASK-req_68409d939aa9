package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/eaglepoint/oops/backend/internal/audit"
	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/eaglepoint/oops/backend/internal/store"
	"github.com/labstack/echo/v4"
)

// Settings namespaces used by the portal. Keeping them as constants in
// Go lets the handler avoid typos from string literals across the
// codebase and makes the set auditable.
const (
	SettingMapImageURL = "map.image.url"
)

// GetMapConfig returns the current service-area map configuration:
// the raster image URL (or data: URI) the OfflineMap component
// renders behind the polygon overlay. A missing setting surfaces as
// an empty string so the UI can fall back to the polygon-only view.
func (s *Server) GetMapConfig(c echo.Context) error {
	v, err := s.Store.GetSetting(c.Request().Context(), SettingMapImageURL)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"map_image_url": v})
}

// AdminPutMapConfig replaces the service-area map-image URL. The
// change is audit-logged. An empty string clears the setting.
func (s *Server) AdminPutMapConfig(c echo.Context) error {
	var body struct {
		MapImageURL string `json:"map_image_url"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	url := strings.TrimSpace(body.MapImageURL)
	// Best-effort validation: allow an absolute `http`/`https` URL, an
	// origin-relative path starting with `/`, or a `data:` URI. Reject
	// anything else so nobody accidentally stores a credential.
	if url != "" && !(strings.HasPrefix(url, "http://") ||
		strings.HasPrefix(url, "https://") ||
		strings.HasPrefix(url, "/") ||
		strings.HasPrefix(url, "data:image/")) {
		return echo.NewHTTPError(http.StatusBadRequest,
			"map_image_url must be http(s)://, /, or data:image/")
	}
	ctx := c.Request().Context()
	before, _ := s.Store.GetSetting(ctx, SettingMapImageURL)
	if err := s.Store.PutSetting(ctx, SettingMapImageURL, url); err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c),
		audit.EntitySystemSettings, SettingMapImageURL, "put", "",
		map[string]string{"value": before},
		map[string]string{"value": url})
	return c.JSON(http.StatusOK, map[string]any{"map_image_url": url})
}
