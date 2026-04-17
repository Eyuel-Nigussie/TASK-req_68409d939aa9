package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/labstack/echo/v4"
)

// CreateCustomer registers a new customer. Identifier and street are
// encrypted by the vault before persistence.
func (s *Server) CreateCustomer(c echo.Context) error {
	var body struct {
		Name       string   `json:"name"`
		Identifier string   `json:"identifier"`
		Street     string   `json:"street"`
		City       string   `json:"city"`
		State      string   `json:"state"`
		ZIP        string   `json:"zip"`
		Phone      string   `json:"phone"`
		Email      string   `json:"email"`
		Tags       []string `json:"tags"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if body.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name required")
	}
	encID, err := s.Vault.Encrypt(body.Identifier)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	encStreet, err := s.Vault.Encrypt(body.Street)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	cu := models.Customer{
		ID:         newID(),
		Name:       body.Name,
		Identifier: encID,
		Street:     encStreet,
		City:       body.City,
		State:      body.State,
		ZIP:        body.ZIP,
		Phone:      body.Phone,
		Email:      body.Email,
		Tags:       body.Tags,
		CreatedAt:  s.Clock(),
		UpdatedAt:  s.Clock(),
	}
	if err := s.Store.CreateCustomer(c.Request().Context(), cu); err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	_ = s.Audit.Log(c.Request().Context(), sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), "customer", cu.ID, "create", "", nil, redactCustomer(cu))
	return c.JSON(http.StatusCreated, s.customerView(cu))
}

// GetCustomer returns a customer with identifiers decrypted for display.
func (s *Server) GetCustomer(c echo.Context) error {
	cu, err := s.Store.GetCustomer(c.Request().Context(), c.Param("id"))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, s.customerView(cu))
}

// SearchCustomers returns ranked suggestions for the global search bar.
func (s *Server) SearchCustomers(c echo.Context) error {
	q := c.QueryParam("q")
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	cs, err := s.Store.SearchCustomers(c.Request().Context(), q, limit)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	views := make([]map[string]any, len(cs))
	for i, cu := range cs {
		views[i] = s.customerView(cu)
	}
	return c.JSON(http.StatusOK, views)
}

// UpdateCustomer edits mutable fields; identifier is NOT updateable via API
// because correcting a patient/customer ID is an administrative action.
func (s *Server) UpdateCustomer(c echo.Context) error {
	ctx := c.Request().Context()
	existing, err := s.Store.GetCustomer(ctx, c.Param("id"))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	var body struct {
		Name   string   `json:"name"`
		Street string   `json:"street"`
		City   string   `json:"city"`
		State  string   `json:"state"`
		ZIP    string   `json:"zip"`
		Phone  string   `json:"phone"`
		Email  string   `json:"email"`
		Tags   []string `json:"tags"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	before := redactCustomer(existing)
	if body.Name != "" {
		existing.Name = body.Name
	}
	if body.Street != "" {
		enc, err := s.Vault.Encrypt(body.Street)
		if err != nil {
			return httpx.WriteError(c, err)
		}
		existing.Street = enc
	}
	if body.City != "" {
		existing.City = body.City
	}
	if body.State != "" {
		existing.State = body.State
	}
	if body.ZIP != "" {
		existing.ZIP = body.ZIP
	}
	if body.Phone != "" {
		existing.Phone = body.Phone
	}
	if body.Email != "" {
		existing.Email = body.Email
	}
	if body.Tags != nil {
		existing.Tags = body.Tags
	}
	existing.UpdatedAt = s.Clock()
	if err := s.Store.UpdateCustomer(ctx, existing); err != nil {
		return httpx.WriteError(c, err)
	}
	sess := httpx.CurrentSession(c)
	_ = s.Audit.Log(ctx, sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), "customer", existing.ID, "update", "", before, redactCustomer(existing))
	return c.JSON(http.StatusOK, s.customerView(existing))
}

// CustomersByAddress looks up customers whose address matches city/ZIP.
// When `street` is provided, each candidate's encrypted street field is
// decrypted in this handler (which holds the vault) and a case-insensitive
// substring match is applied. Without this, the store layer would be
// comparing against ciphertext and would silently fail.
func (s *Server) CustomersByAddress(c echo.Context) error {
	street := strings.TrimSpace(c.QueryParam("street"))
	city := c.QueryParam("city")
	zip := c.QueryParam("zip")
	if zip == "" && city == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "must specify at least city or zip")
	}
	cs, err := s.Store.FindByAddress(c.Request().Context(), street, city, zip)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	needle := strings.ToLower(street)
	views := make([]map[string]any, 0, len(cs))
	for _, cu := range cs {
		plain, err := s.Vault.Decrypt(cu.Street)
		if err != nil {
			// Can't decrypt — skip rather than return partial garbage.
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(plain), needle) {
			continue
		}
		v := s.customerView(cu)
		// customerView already decrypts Street; make sure we use the value
		// we just verified in case the vault is swapped in the future.
		v["street"] = plain
		views = append(views, v)
	}
	return c.JSON(http.StatusOK, views)
}

// customerView decrypts sensitive fields and returns a response-safe map.
func (s *Server) customerView(cu models.Customer) map[string]any {
	id, _ := s.Vault.Decrypt(cu.Identifier)
	street, _ := s.Vault.Decrypt(cu.Street)
	return map[string]any{
		"id":         cu.ID,
		"name":       cu.Name,
		"identifier": id,
		"street":     street,
		"city":       cu.City,
		"state":      cu.State,
		"zip":        cu.ZIP,
		"phone":      cu.Phone,
		"email":      cu.Email,
		"tags":       cu.Tags,
		"created_at": cu.CreatedAt,
		"updated_at": cu.UpdatedAt,
	}
}

// redactCustomer produces an audit-safe snapshot that replaces encrypted
// fields with placeholder text so the audit log never stores plaintext PII.
func redactCustomer(cu models.Customer) map[string]any {
	return map[string]any{
		"id":    cu.ID,
		"name":  cu.Name,
		"city":  cu.City,
		"state": cu.State,
		"zip":   cu.ZIP,
		"phone": cu.Phone,
		"email": cu.Email,
		"tags":  cu.Tags,
		"sensitive_fields": map[string]bool{
			"identifier": cu.Identifier != "",
			"street":     cu.Street != "",
		},
	}
}
