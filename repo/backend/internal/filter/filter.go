// Package filter defines the "advanced filter" query object that the UI
// submits for saved filters and exports. The validator rejects overly broad
// filters to prevent analysts from accidentally dumping the entire dataset.
package filter

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Errors surfaced by Validate so callers can map them to HTTP status codes.
var (
	ErrTooBroad       = errors.New("filter is too broad; narrow keyword, date, or status")
	ErrBadDate        = errors.New("dates must be formatted MM/DD/YYYY")
	ErrDateOrder      = errors.New("end date must be on or after start date")
	ErrBadPriceRange  = errors.New("max price must be >= min price and both >= 0")
	ErrBadPage        = errors.New("page and size must be positive; size <= 500")
	ErrUnknownSort    = errors.New("unknown sort field")
	ErrUnknownStatus  = errors.New("unknown status value")
	ErrUnknownEntity  = errors.New("unknown entity type")
)

// Supported entity types (table scope).
const (
	EntityCustomer = "customer"
	EntityOrder    = "order"
	EntitySample   = "sample"
	EntityReport   = "report"
)

var knownEntities = map[string]struct{}{
	EntityCustomer: {}, EntityOrder: {}, EntitySample: {}, EntityReport: {},
}

// Allowed sort keys per entity. Restricting this list forestalls injection
// and prevents ORDER BY on unindexed columns.
var allowedSort = map[string]map[string]struct{}{
	EntityCustomer: {"name": {}, "created_at": {}, "updated_at": {}},
	EntityOrder:    {"placed_at": {}, "status": {}, "total_cents": {}, "priority": {}},
	EntitySample:   {"collected_at": {}, "status": {}},
	EntityReport:   {"reported_at": {}, "version": {}, "status": {}},
}

// Filter is the full JSON body submitted from the UI. All fields are
// optional; validation also checks cross-field consistency.
type Filter struct {
	Entity       string   `json:"entity"`
	Keyword      string   `json:"keyword,omitempty"`
	Statuses     []string `json:"statuses,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Priority     string   `json:"priority,omitempty"`
	StartDate    string   `json:"start_date,omitempty"` // MM/DD/YYYY inclusive
	EndDate      string   `json:"end_date,omitempty"`   // MM/DD/YYYY inclusive
	MinPriceUSD  *float64 `json:"min_price_usd,omitempty"`
	MaxPriceUSD  *float64 `json:"max_price_usd,omitempty"`
	SortBy       string   `json:"sort_by,omitempty"`
	SortDesc     bool     `json:"sort_desc,omitempty"`
	Page         int      `json:"page"`
	Size         int      `json:"size"`
}

// MaxExportSize is the largest page size that can be used for an export.
// Saved-filter validation requires at least one narrowing criterion (keyword,
// status, or date range) whenever size exceeds this limit.
const (
	MaxExportSize = 500
	DefaultSize   = 25
)

// ParseDate parses MM/DD/YYYY. Returns a zero time if blank.
func ParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse("01/02/2006", s)
	if err != nil {
		return time.Time{}, ErrBadDate
	}
	return t, nil
}

// Validate returns nil iff the filter is well-formed and not overly broad.
// It also normalizes defaults for Page/Size and sort fields.
func (f *Filter) Validate(knownStatuses []string) error {
	if _, ok := knownEntities[f.Entity]; !ok {
		return fmt.Errorf("%w: %q", ErrUnknownEntity, f.Entity)
	}
	if _, err := ParseDate(f.StartDate); err != nil {
		return err
	}
	if _, err := ParseDate(f.EndDate); err != nil {
		return err
	}
	start, _ := ParseDate(f.StartDate)
	end, _ := ParseDate(f.EndDate)
	if !start.IsZero() && !end.IsZero() && end.Before(start) {
		return ErrDateOrder
	}
	if f.MinPriceUSD != nil && *f.MinPriceUSD < 0 {
		return ErrBadPriceRange
	}
	if f.MaxPriceUSD != nil && *f.MaxPriceUSD < 0 {
		return ErrBadPriceRange
	}
	if f.MinPriceUSD != nil && f.MaxPriceUSD != nil && *f.MaxPriceUSD < *f.MinPriceUSD {
		return ErrBadPriceRange
	}
	if len(knownStatuses) > 0 {
		known := make(map[string]struct{}, len(knownStatuses))
		for _, s := range knownStatuses {
			known[s] = struct{}{}
		}
		for _, s := range f.Statuses {
			if _, ok := known[s]; !ok {
				return fmt.Errorf("%w: %q", ErrUnknownStatus, s)
			}
		}
	}
	if f.SortBy != "" {
		sorts, ok := allowedSort[f.Entity]
		if !ok {
			return ErrUnknownEntity
		}
		if _, ok := sorts[f.SortBy]; !ok {
			return fmt.Errorf("%w: %q for %s", ErrUnknownSort, f.SortBy, f.Entity)
		}
	}
	if f.Page == 0 {
		f.Page = 1
	}
	if f.Size == 0 {
		f.Size = DefaultSize
	}
	if f.Page < 1 || f.Size < 1 || f.Size > MaxExportSize {
		return ErrBadPage
	}
	// "Too broad" check — applies whenever the size pushes toward export.
	if f.Size > 100 {
		if strings.TrimSpace(f.Keyword) == "" &&
			len(f.Statuses) == 0 &&
			len(f.Tags) == 0 &&
			f.StartDate == "" && f.EndDate == "" &&
			f.MinPriceUSD == nil && f.MaxPriceUSD == nil {
			return ErrTooBroad
		}
	}
	return nil
}

// CanonicalKey returns a deterministic key for a saved filter so callers
// can dedupe identical filters in the per-user library.
func (f *Filter) CanonicalKey() string {
	statuses := append([]string(nil), f.Statuses...)
	sort.Strings(statuses)
	tags := append([]string(nil), f.Tags...)
	sort.Strings(tags)
	var min, max string
	if f.MinPriceUSD != nil {
		min = fmt.Sprintf("%.2f", *f.MinPriceUSD)
	}
	if f.MaxPriceUSD != nil {
		max = fmt.Sprintf("%.2f", *f.MaxPriceUSD)
	}
	return strings.Join([]string{
		f.Entity,
		strings.ToLower(strings.TrimSpace(f.Keyword)),
		strings.Join(statuses, ","),
		strings.Join(tags, ","),
		f.Priority,
		f.StartDate,
		f.EndDate,
		min, max,
		f.SortBy,
		fmt.Sprintf("%v", f.SortDesc),
	}, "|")
}
