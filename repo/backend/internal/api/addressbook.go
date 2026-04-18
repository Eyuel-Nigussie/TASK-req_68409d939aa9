package api

import (
	"net/http"

	"github.com/eaglepoint/oops/backend/internal/audit"
	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/labstack/echo/v4"
)

// ListAddressBook returns the caller's saved addresses.
func (s *Server) ListAddressBook(c echo.Context) error {
	sess := httpx.CurrentSession(c)
	out, err := s.Store.ListAddresses(c.Request().Context(), sess.UserID)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	views := make([]map[string]any, len(out))
	for i, a := range out {
		street, _ := s.Vault.Decrypt(a.Street)
		views[i] = map[string]any{
			"id": a.ID, "label": a.Label, "customer_id": a.CustomerID,
			"street": street, "city": a.City, "state": a.State, "zip": a.ZIP,
			"lat": a.Lat, "lng": a.Lng, "created_at": a.CreatedAt,
		}
	}
	return c.JSON(http.StatusOK, views)
}

// CreateAddressBookEntry saves a labeled address for the current user.
func (s *Server) CreateAddressBookEntry(c echo.Context) error {
	var body struct {
		Label      string  `json:"label"`
		CustomerID string  `json:"customer_id"`
		Street     string  `json:"street"`
		City       string  `json:"city"`
		State      string  `json:"state"`
		ZIP        string  `json:"zip"`
		Lat        float64 `json:"lat"`
		Lng        float64 `json:"lng"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if body.Label == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "label required")
	}
	enc, err := s.Vault.Encrypt(body.Street)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	a := models.AddressBookEntry{
		ID:         newID(),
		OwnerID:    sess.UserID,
		CustomerID: body.CustomerID,
		Label:      body.Label,
		Street:     enc,
		City:       body.City,
		State:      body.State,
		ZIP:        body.ZIP,
		Lat:        body.Lat,
		Lng:        body.Lng,
		CreatedAt:  s.Clock(),
	}
	ctx := c.Request().Context()
	if err := s.Store.CreateAddress(ctx, a); err != nil {
		return httpx.WriteError(c, err)
	}
	// Record an audit entry with encrypted fields redacted, so the log
	// tracks the operation without copying plaintext addresses into it.
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityAddressBook, a.ID, "create", "",
		nil, map[string]any{
			"label": a.Label, "city": a.City, "state": a.State, "zip": a.ZIP,
			"customer_id": a.CustomerID, "has_street": a.Street != "",
		})
	return c.JSON(http.StatusCreated, map[string]any{"id": a.ID, "label": a.Label})
}

// DeleteAddressBookEntry removes one owned entry.
func (s *Server) DeleteAddressBookEntry(c echo.Context) error {
	sess := httpx.CurrentSession(c)
	ctx := c.Request().Context()
	id := c.Param("id")
	// Look up first so the audit can capture a redacted snapshot.
	var before map[string]any
	if list, err := s.Store.ListAddresses(ctx, sess.UserID); err == nil {
		for _, a := range list {
			if a.ID == id {
				before = map[string]any{
					"label": a.Label, "city": a.City, "state": a.State, "zip": a.ZIP,
				}
				break
			}
		}
	}
	if err := s.Store.DeleteAddress(ctx, sess.UserID, id); err != nil {
		return httpx.WriteError(c, err)
	}
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityAddressBook, id, "delete", "", before, nil)
	return c.NoContent(http.StatusNoContent)
}
