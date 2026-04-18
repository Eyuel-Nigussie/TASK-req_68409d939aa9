package api

import (
	"net/http"

	"github.com/eaglepoint/oops/backend/internal/audit"
	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/labstack/echo/v4"
)

// AdminListPermissions returns the full permission catalog. This is what
// the admin UI renders as the list of checkboxes.
func (s *Server) AdminListPermissions(c echo.Context) error {
	perms, err := s.Store.ListPermissions(c.Request().Context())
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, perms)
}

// AdminListRolePermissions returns every (role -> permission) grant so
// the UI can render the role/permission matrix.
func (s *Server) AdminListRolePermissions(c echo.Context) error {
	rp, err := s.Store.ListRolePermissions(c.Request().Context())
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, rp)
}

// AdminPutRolePermissions replaces the grant set for one role atomically.
// Body: {"permission_ids": ["orders.write", ...]}
func (s *Server) AdminPutRolePermissions(c echo.Context) error {
	var body struct {
		PermissionIDs []string `json:"permission_ids"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	role := c.Param("role")
	if role == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "role required")
	}
	ctx := c.Request().Context()
	all, _ := s.Store.ListRolePermissions(ctx)
	var before []models.RolePermission
	for _, rp := range all {
		if rp.Role == role {
			before = append(before, rp)
		}
	}
	if err := s.Store.SetRolePermissions(ctx, role, body.PermissionIDs); err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c),
		audit.EntityRolePermissions, role, "replace", "", before, body.PermissionIDs)
	return c.JSON(http.StatusOK, map[string]any{"role": role, "permissions": body.PermissionIDs})
}

// AdminListUserPermissions returns the individual grants on a single user.
func (s *Server) AdminListUserPermissions(c echo.Context) error {
	ids, err := s.Store.ListUserPermissions(c.Request().Context(), c.Param("id"))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, ids)
}

// AdminPutUserPermissions replaces the individual grants on a single user.
func (s *Server) AdminPutUserPermissions(c echo.Context) error {
	var body struct {
		PermissionIDs []string `json:"permission_ids"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	userID := c.Param("id")
	before, _ := s.Store.ListUserPermissions(ctx, userID)
	if err := s.Store.SetUserPermissions(ctx, userID, body.PermissionIDs); err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c),
		audit.EntityUserPermissions, userID, "replace", "", before, body.PermissionIDs)
	return c.JSON(http.StatusOK, map[string]any{"user_id": userID, "permissions": body.PermissionIDs})
}

