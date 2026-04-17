package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/eaglepoint/oops/backend/internal/geo"
	"github.com/eaglepoint/oops/backend/internal/lab"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/order"
	"github.com/eaglepoint/oops/backend/internal/search"
)

// Memory is an in-memory implementation of Store used for tests and for a
// "quickstart" mode on a fresh deployment. It is goroutine-safe. Data does
// not survive a process restart.
type Memory struct {
	mu sync.RWMutex

	users      map[string]models.User
	usersByName map[string]string
	customers  map[string]models.Customer
	addresses  map[string]models.AddressBookEntry
	regions    []geo.Region
	orders     map[string]order.Order
	samples    map[string]lab.Sample
	reports    map[string]lab.Report
	savedFilters map[string]models.SavedFilter
	audit      []models.AuditEntry
	exceptions map[string]order.Exception
	refRanges  []lab.RefRange
	routes     []RouteRow
	// permission state
	permCatalog map[string]models.Permission
	rolePerms   map[string]map[string]struct{} // role -> perm ids
	userPerms   map[string]map[string]struct{} // userID -> perm ids
	loginAttempts map[string]models.LoginAttempt
	testItems     map[string][]models.TestItem // sampleID -> items
	settings      map[string]string
}

// NewMemory constructs an empty memory store. Default permission grants
// matching migrations/0001_init.sql are seeded so tests and quickstart
// deployments have a working authorization policy out of the box.
func NewMemory() *Memory {
	m := &Memory{
		users:         make(map[string]models.User),
		usersByName:   make(map[string]string),
		customers:     make(map[string]models.Customer),
		addresses:     make(map[string]models.AddressBookEntry),
		orders:        make(map[string]order.Order),
		samples:       make(map[string]lab.Sample),
		reports:       make(map[string]lab.Report),
		savedFilters:  make(map[string]models.SavedFilter),
		exceptions:    make(map[string]order.Exception),
		permCatalog:   make(map[string]models.Permission),
		rolePerms:     make(map[string]map[string]struct{}),
		userPerms:     make(map[string]map[string]struct{}),
		loginAttempts: make(map[string]models.LoginAttempt),
		testItems:     make(map[string][]models.TestItem),
		settings:      make(map[string]string),
	}
	m.seedDefaultPermissions()
	return m
}

// seedDefaultPermissions installs the same permission catalog and role
// grants that the SQL migration provides. The two must stay in sync so a
// fresh deployment behaves identically under Postgres and memory.
func (m *Memory) seedDefaultPermissions() {
	catalog := []models.Permission{
		{ID: "customers.read", Description: "View customer records"},
		{ID: "customers.write", Description: "Create or edit customers"},
		{ID: "orders.read", Description: "View orders"},
		{ID: "orders.write", Description: "Create or transition orders"},
		{ID: "orders.refund", Description: "Refund an order"},
		{ID: "samples.read", Description: "View samples"},
		{ID: "samples.write", Description: "Create or transition samples"},
		{ID: "reports.read", Description: "View reports"},
		{ID: "reports.write", Description: "Create and correct reports"},
		{ID: "reports.archive", Description: "Archive reports"},
		{ID: "dispatch.validate", Description: "Validate dispatch pins & quote fees"},
		{ID: "dispatch.configure", Description: "Edit service regions and route table"},
		{ID: "analytics.view", Description: "View operational analytics"},
		{ID: "orders.export", Description: "Export orders to CSV (bounded filter)"},
		{ID: "admin.users", Description: "Manage users and permissions"},
		{ID: "admin.reference", Description: "Edit reference ranges"},
		{ID: "admin.audit", Description: "View audit log"},
		{ID: "admin.settings", Description: "Edit system settings (map image, etc.)"},
	}
	for _, p := range catalog {
		m.permCatalog[p.ID] = p
	}
	grants := map[string][]string{
		"front_desk": {"customers.read", "customers.write", "orders.read", "orders.write"},
		"lab_tech":   {"samples.read", "samples.write", "reports.read", "reports.write", "reports.archive"},
		"dispatch":   {"orders.read", "dispatch.validate", "customers.read"},
		"analyst":    {"customers.read", "orders.read", "samples.read", "reports.read", "analytics.view", "orders.export"},
		"admin": {
			"customers.read", "customers.write",
			"orders.read", "orders.write", "orders.refund", "orders.export",
			"samples.read", "samples.write",
			"reports.read", "reports.write", "reports.archive",
			"dispatch.validate", "dispatch.configure",
			"analytics.view", "admin.users", "admin.reference", "admin.audit",
			"admin.settings",
		},
	}
	for role, ids := range grants {
		set := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			set[id] = struct{}{}
		}
		m.rolePerms[role] = set
	}
}

// ---------- Users ----------

func (m *Memory) CreateUser(_ context.Context, u models.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.usersByName[u.Username]; ok {
		return ErrConflict
	}
	m.users[u.ID] = u
	m.usersByName[u.Username] = u.ID
	return nil
}

func (m *Memory) GetUserByUsername(_ context.Context, name string) (models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.usersByName[name]
	if !ok {
		return models.User{}, ErrNotFound
	}
	return m.users[id], nil
}

func (m *Memory) GetUserByID(_ context.Context, id string) (models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[id]
	if !ok {
		return models.User{}, ErrNotFound
	}
	return u, nil
}

func (m *Memory) ListUsers(_ context.Context) ([]models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]models.User, 0, len(m.users))
	for _, u := range m.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out, nil
}

func (m *Memory) UpdateUser(_ context.Context, u models.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[u.ID]; !ok {
		return ErrNotFound
	}
	m.users[u.ID] = u
	m.usersByName[u.Username] = u.ID
	return nil
}

// ---------- Customers ----------

func (m *Memory) CreateCustomer(_ context.Context, c models.Customer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.customers[c.ID]; ok {
		return ErrConflict
	}
	m.customers[c.ID] = c
	return nil
}

func (m *Memory) GetCustomer(_ context.Context, id string) (models.Customer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.customers[id]
	if !ok {
		return models.Customer{}, ErrNotFound
	}
	return c, nil
}

func (m *Memory) UpdateCustomer(_ context.Context, c models.Customer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.customers[c.ID]; !ok {
		return ErrNotFound
	}
	m.customers[c.ID] = c
	return nil
}

func (m *Memory) SearchCustomers(_ context.Context, query string, limit int) ([]models.Customer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cands := make([]search.Suggestion, 0, len(m.customers))
	for _, c := range m.customers {
		label := c.Name + " " + c.Email + " " + c.Phone + " " + c.Street + " " + c.City + " " + c.ZIP
		cands = append(cands, search.Suggestion{ID: c.ID, Label: label, Kind: "customer"})
	}
	ranked := search.Rank(query, cands, 0.25, limit)
	out := make([]models.Customer, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, m.customers[r.ID])
	}
	return out, nil
}

// FindByAddress filters on the non-encrypted address columns (city + ZIP).
// Street-substring filtering is NOT performed here because Street is stored
// as an encryption envelope whose bytes don't carry the plaintext. Callers
// that need to narrow by street must decrypt the returned rows in the
// service layer (which holds the vault) and filter in Go.
func (m *Memory) FindByAddress(_ context.Context, _street, city, zip string) ([]models.Customer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []models.Customer
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	for _, c := range m.customers {
		if zip != "" && norm(c.ZIP) != norm(zip) {
			continue
		}
		if city != "" && norm(c.City) != norm(city) {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ---------- Address book ----------

func (m *Memory) CreateAddress(_ context.Context, a models.AddressBookEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addresses[a.ID] = a
	return nil
}

func (m *Memory) ListAddresses(_ context.Context, ownerID string) ([]models.AddressBookEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []models.AddressBookEntry
	for _, a := range m.addresses {
		if a.OwnerID == ownerID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out, nil
}

func (m *Memory) DeleteAddress(_ context.Context, ownerID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.addresses[id]
	if !ok || a.OwnerID != ownerID {
		return ErrNotFound
	}
	delete(m.addresses, id)
	return nil
}

// ---------- Service areas ----------

func (m *Memory) ListRegions(_ context.Context) ([]geo.Region, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]geo.Region(nil), m.regions...), nil
}

func (m *Memory) ReplaceRegions(_ context.Context, regions []geo.Region) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.regions = append([]geo.Region(nil), regions...)
	return nil
}

// ---------- Orders ----------

func (m *Memory) CreateOrder(_ context.Context, o order.Order) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.orders[o.ID]; ok {
		return ErrConflict
	}
	m.orders[o.ID] = o
	return nil
}

func (m *Memory) GetOrder(_ context.Context, id string) (order.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	o, ok := m.orders[id]
	if !ok {
		return order.Order{}, ErrNotFound
	}
	return o, nil
}

func (m *Memory) UpdateOrder(_ context.Context, o order.Order) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.orders[o.ID]; !ok {
		return ErrNotFound
	}
	m.orders[o.ID] = o
	return nil
}

func (m *Memory) AppendOrderEvent(_ context.Context, ev order.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.orders[ev.OrderID]
	if !ok {
		return ErrNotFound
	}
	o.Events = append(o.Events, ev)
	m.orders[ev.OrderID] = o
	return nil
}

func (m *Memory) ListOrders(_ context.Context, statuses []string, from, to *int64, limit, offset int) ([]order.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	statusSet := make(map[string]struct{}, len(statuses))
	for _, s := range statuses {
		statusSet[s] = struct{}{}
	}
	var out []order.Order
	for _, o := range m.orders {
		if len(statusSet) > 0 {
			if _, ok := statusSet[string(o.Status)]; !ok {
				continue
			}
		}
		ts := o.PlacedAt.Unix()
		if from != nil && ts < *from {
			continue
		}
		if to != nil && ts > *to {
			continue
		}
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PlacedAt.After(out[j].PlacedAt) })
	if offset > len(out) {
		return nil, nil
	}
	out = out[offset:]
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// QueryOrders evaluates the full filter payload against the in-memory set.
// This mirrors the semantics that the Postgres implementation provides via
// SQL so handlers can be tested against the same contract in both stores.
func (m *Memory) QueryOrders(_ context.Context, q OrderQuery) ([]order.Order, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	statusSet := map[string]struct{}{}
	for _, s := range q.Statuses {
		statusSet[s] = struct{}{}
	}
	tagSet := map[string]struct{}{}
	for _, t := range q.Tags {
		tagSet[t] = struct{}{}
	}
	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	var matched []order.Order
	for _, o := range m.orders {
		if len(statusSet) > 0 {
			if _, ok := statusSet[string(o.Status)]; !ok {
				continue
			}
		}
		if q.Priority != "" && o.Priority != q.Priority {
			continue
		}
		if q.StartUnix != nil && o.PlacedAt.Unix() < *q.StartUnix {
			continue
		}
		if q.EndUnix != nil && o.PlacedAt.Unix() > *q.EndUnix {
			continue
		}
		if q.MinCents != nil && o.TotalCents < *q.MinCents {
			continue
		}
		if q.MaxCents != nil && o.TotalCents > *q.MaxCents {
			continue
		}
		if len(tagSet) > 0 {
			found := false
			for _, t := range o.Tags {
				if _, ok := tagSet[t]; ok {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if kw != "" {
			hay := strings.ToLower(o.ID + " " + string(o.Status) + " " + o.Priority + " " + o.CustomerID + " " + strings.Join(o.Tags, " "))
			if !strings.Contains(hay, kw) {
				continue
			}
		}
		matched = append(matched, o)
	}
	total := len(matched)
	// Sorting.
	less := func(i, j int) bool { return matched[i].PlacedAt.After(matched[j].PlacedAt) }
	switch q.SortBy {
	case "placed_at":
		less = func(i, j int) bool { return matched[i].PlacedAt.Before(matched[j].PlacedAt) }
	case "status":
		less = func(i, j int) bool { return matched[i].Status < matched[j].Status }
	case "total_cents":
		less = func(i, j int) bool { return matched[i].TotalCents < matched[j].TotalCents }
	case "priority":
		less = func(i, j int) bool { return matched[i].Priority < matched[j].Priority }
	}
	if q.SortDesc && q.SortBy != "" {
		orig := less
		less = func(i, j int) bool { return !orig(i, j) }
	}
	sort.Slice(matched, less)
	// Pagination.
	if q.Offset > len(matched) {
		return nil, total, nil
	}
	page := matched[q.Offset:]
	if q.Limit > 0 && len(page) > q.Limit {
		page = page[:q.Limit]
	}
	return page, total, nil
}

// OrdersByAddress returns orders whose delivery address matches city/ZIP.
// Street is encrypted in storage, so street-substring matching happens in
// the handler layer after decryption.
func (m *Memory) OrdersByAddress(_ context.Context, city, zip string) ([]order.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	var out []order.Order
	for _, o := range m.orders {
		if zip != "" && norm(o.DeliveryZIP) != norm(zip) {
			continue
		}
		if city != "" && norm(o.DeliveryCity) != norm(city) {
			continue
		}
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PlacedAt.After(out[j].PlacedAt) })
	return out, nil
}

func (m *Memory) ListExceptions(_ context.Context) ([]order.Exception, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]order.Exception, 0, len(m.exceptions))
	for _, e := range m.exceptions {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DetectedAt.Before(out[j].DetectedAt) })
	return out, nil
}

func (m *Memory) PutException(_ context.Context, ex order.Exception) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exceptions[ex.OrderID+"|"+ex.Kind] = ex
	return nil
}

// ---------- Samples ----------

func (m *Memory) CreateSample(_ context.Context, s lab.Sample) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.samples[s.ID]; ok {
		return ErrConflict
	}
	m.samples[s.ID] = s
	return nil
}

func (m *Memory) GetSample(_ context.Context, id string) (lab.Sample, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.samples[id]
	if !ok {
		return lab.Sample{}, ErrNotFound
	}
	return s, nil
}

func (m *Memory) UpdateSample(_ context.Context, s lab.Sample) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.samples[s.ID]; !ok {
		return ErrNotFound
	}
	m.samples[s.ID] = s
	return nil
}

func (m *Memory) ListSamples(_ context.Context, statuses []string, limit, offset int) ([]lab.Sample, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	set := make(map[string]struct{}, len(statuses))
	for _, s := range statuses {
		set[s] = struct{}{}
	}
	var out []lab.Sample
	for _, s := range m.samples {
		if len(set) > 0 {
			if _, ok := set[string(s.Status)]; !ok {
				continue
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CollectedAt.After(out[j].CollectedAt) })
	if offset > len(out) {
		return nil, nil
	}
	out = out[offset:]
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---------- Reports ----------

func (m *Memory) CreateReport(_ context.Context, r lab.Report) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.reports[r.ID]; ok {
		return ErrConflict
	}
	m.reports[r.ID] = r
	return nil
}

func (m *Memory) GetReport(_ context.Context, id string) (lab.Report, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.reports[id]
	if !ok {
		return lab.Report{}, ErrNotFound
	}
	return r, nil
}

func (m *Memory) LatestReportForSample(_ context.Context, sampleID string) (lab.Report, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var latest *lab.Report
	for _, r := range m.reports {
		if r.SampleID != sampleID {
			continue
		}
		if latest == nil || r.Version > latest.Version {
			cp := r
			latest = &cp
		}
	}
	if latest == nil {
		return lab.Report{}, ErrNotFound
	}
	return *latest, nil
}

func (m *Memory) UpdateReport(_ context.Context, r lab.Report) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.reports[r.ID]; !ok {
		return ErrNotFound
	}
	m.reports[r.ID] = r
	return nil
}

// ReplaceWithCorrection atomically updates the prior row (to superseded) and
// inserts the new version. In the in-memory store this is straightforward;
// the Postgres impl uses a transaction.
func (m *Memory) ReplaceWithCorrection(_ context.Context, old, next lab.Report) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reports[old.ID] = old
	m.reports[next.ID] = next
	return nil
}

func (m *Memory) SearchReports(_ context.Context, query string, limit int) ([]lab.Report, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cands := make([]search.Suggestion, 0, len(m.reports))
	for _, r := range m.reports {
		cands = append(cands, search.Suggestion{ID: r.ID, Label: r.SearchText, Kind: "report"})
	}
	ranked := search.Rank(query, cands, 0.2, limit)
	out := make([]lab.Report, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, m.reports[r.ID])
	}
	return out, nil
}

func (m *Memory) ListReports(_ context.Context, limit, offset int) ([]lab.Report, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Exclude archived reports from the default list; they remain available
	// via search and via the explicit archive-listing query.
	out := make([]lab.Report, 0, len(m.reports))
	for _, r := range m.reports {
		if r.IsArchived() {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	if offset > len(out) {
		return nil, nil
	}
	out = out[offset:]
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ListArchivedReports returns reports whose ArchivedAt is non-zero, sorted
// newest-archived first.
func (m *Memory) ListArchivedReports(_ context.Context) ([]lab.Report, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []lab.Report
	for _, r := range m.reports {
		if r.IsArchived() {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ArchivedAt.After(out[j].ArchivedAt) })
	return out, nil
}

// ---------- Saved filters ----------

func (m *Memory) CreateSavedFilter(_ context.Context, f models.SavedFilter) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Dedupe on (owner, canonical key).
	for _, existing := range m.savedFilters {
		if existing.OwnerID == f.OwnerID && existing.Key == f.Key {
			return ErrConflict
		}
	}
	m.savedFilters[f.ID] = f
	return nil
}

func (m *Memory) ListSavedFilters(_ context.Context, ownerID string) ([]models.SavedFilter, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []models.SavedFilter
	for _, f := range m.savedFilters {
		if f.OwnerID == ownerID {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) DeleteSavedFilter(_ context.Context, ownerID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.savedFilters[id]
	if !ok || f.OwnerID != ownerID {
		return ErrNotFound
	}
	delete(m.savedFilters, id)
	return nil
}

// ---------- Reference ranges ----------

func (m *Memory) ListRefRanges(_ context.Context) ([]lab.RefRange, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]lab.RefRange(nil), m.refRanges...), nil
}

func (m *Memory) ReplaceRefRanges(_ context.Context, rr []lab.RefRange) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refRanges = append([]lab.RefRange(nil), rr...)
	return nil
}

// ---------- Route table ----------

func (m *Memory) ListRoutes(_ context.Context) ([]RouteRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]RouteRow(nil), m.routes...), nil
}

func (m *Memory) ReplaceRoutes(_ context.Context, rows []RouteRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes = append([]RouteRow(nil), rows...)
	return nil
}

// ---------- TestItems ----------

func (m *Memory) ListTestItems(_ context.Context, sampleID string) ([]models.TestItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := append([]models.TestItem(nil), m.testItems[sampleID]...)
	return out, nil
}

func (m *Memory) ReplaceTestItems(_ context.Context, sampleID string, items []models.TestItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(items) == 0 {
		delete(m.testItems, sampleID)
		return nil
	}
	cp := make([]models.TestItem, len(items))
	copy(cp, items)
	m.testItems[sampleID] = cp
	return nil
}

// ---------- SystemSettings ----------

func (m *Memory) GetSetting(_ context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.settings[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (m *Memory) PutSetting(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[key] = value
	return nil
}

func (m *Memory) ListSettings(_ context.Context) (map[string]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]string, len(m.settings))
	for k, v := range m.settings {
		out[k] = v
	}
	return out, nil
}

// ---------- Permissions ----------

func (m *Memory) ListPermissions(_ context.Context) ([]models.Permission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]models.Permission, 0, len(m.permCatalog))
	for _, p := range m.permCatalog {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *Memory) UpsertPermission(_ context.Context, p models.Permission) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.permCatalog[p.ID] = p
	return nil
}

func (m *Memory) ListRolePermissions(_ context.Context) ([]models.RolePermission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []models.RolePermission
	for role, set := range m.rolePerms {
		for pid := range set {
			out = append(out, models.RolePermission{Role: role, PermissionID: pid})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		return out[i].PermissionID < out[j].PermissionID
	})
	return out, nil
}

func (m *Memory) SetRolePermissions(_ context.Context, role string, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Validate every id exists in the catalog.
	for _, id := range ids {
		if _, ok := m.permCatalog[id]; !ok {
			return fmt.Errorf("unknown permission %q", id)
		}
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	m.rolePerms[role] = set
	return nil
}

func (m *Memory) GrantsForUser(_ context.Context, userID, role string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	merged := make(map[string]struct{})
	if set, ok := m.rolePerms[role]; ok {
		for p := range set {
			merged[p] = struct{}{}
		}
	}
	if set, ok := m.userPerms[userID]; ok {
		for p := range set {
			merged[p] = struct{}{}
		}
	}
	out := make([]string, 0, len(merged))
	for p := range merged {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

func (m *Memory) ListUserPermissions(_ context.Context, userID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	set := m.userPerms[userID]
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

func (m *Memory) SetUserPermissions(_ context.Context, userID string, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if _, ok := m.permCatalog[id]; !ok {
			return fmt.Errorf("unknown permission %q", id)
		}
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	m.userPerms[userID] = set
	return nil
}

// ---------- Login attempts ----------

func (m *Memory) GetLoginAttempt(_ context.Context, username string) (models.LoginAttempt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.loginAttempts[username]
	if !ok {
		return models.LoginAttempt{Username: username}, ErrNotFound
	}
	return a, nil
}

func (m *Memory) UpsertLoginAttempt(_ context.Context, a models.LoginAttempt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loginAttempts[a.Username] = a
	return nil
}

func (m *Memory) ClearLoginAttempt(_ context.Context, username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.loginAttempts, username)
	return nil
}

// ---------- Analytics ----------

func (m *Memory) OrderStatusCounts(_ context.Context, fromUnix, toUnix int64) (map[string]int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]int)
	for _, o := range m.orders {
		ts := o.PlacedAt.Unix()
		if fromUnix > 0 && ts < fromUnix {
			continue
		}
		if toUnix > 0 && ts > toUnix {
			continue
		}
		out[string(o.Status)]++
	}
	return out, nil
}

func (m *Memory) OrdersPerDay(_ context.Context, fromUnix, toUnix int64) ([]AnalyticsDayCount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	bucket := make(map[string]int)
	for _, o := range m.orders {
		ts := o.PlacedAt.Unix()
		if fromUnix > 0 && ts < fromUnix {
			continue
		}
		if toUnix > 0 && ts > toUnix {
			continue
		}
		day := o.PlacedAt.UTC().Format("2006-01-02")
		bucket[day]++
	}
	out := make([]AnalyticsDayCount, 0, len(bucket))
	for d, n := range bucket {
		out = append(out, AnalyticsDayCount{Day: d, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out, nil
}

func (m *Memory) SampleStatusCounts(_ context.Context, fromUnix, toUnix int64) (map[string]int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]int)
	for _, s := range m.samples {
		ts := s.CollectedAt.Unix()
		if fromUnix > 0 && ts < fromUnix {
			continue
		}
		if toUnix > 0 && ts > toUnix {
			continue
		}
		out[string(s.Status)]++
	}
	return out, nil
}

func (m *Memory) AbnormalReportRate(_ context.Context, fromUnix, toUnix int64) (AnalyticsAbnormalRate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var total, abnormal int
	for _, r := range m.reports {
		if !r.IssuedAt.IsZero() {
			ts := r.IssuedAt.Unix()
			if fromUnix > 0 && ts < fromUnix {
				continue
			}
			if toUnix > 0 && ts > toUnix {
				continue
			}
		}
		for _, ms := range r.Measurements {
			total++
			if ms.Flag.IsAbnormal() {
				abnormal++
			}
		}
	}
	var rate float64
	if total > 0 {
		rate = float64(abnormal) / float64(total)
	}
	return AnalyticsAbnormalRate{TotalMeasurements: total, AbnormalMeasurements: abnormal, Rate: rate}, nil
}

func (m *Memory) ExceptionCountsByKind(_ context.Context) (map[string]int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]int)
	for _, e := range m.exceptions {
		out[e.Kind]++
	}
	return out, nil
}

// ---------- Audit ----------

func (m *Memory) AppendAudit(_ context.Context, e models.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Audit log is append-only; no existing entries are modified.
	m.audit = append(m.audit, e)
	return nil
}

func (m *Memory) ListAudit(_ context.Context, entity, entityID string, limit int) ([]models.AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []models.AuditEntry
	for _, e := range m.audit {
		if entity != "" && e.Entity != entity {
			continue
		}
		if entityID != "" && e.EntityID != entityID {
			continue
		}
		out = append(out, e)
	}
	// Chronological order.
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}
