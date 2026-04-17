package lab

import (
	"errors"
	"strings"
	"time"
)

// ReportStatus tracks whether the report is draft, issued, or superseded.
type ReportStatus string

const (
	ReportDraft      ReportStatus = "draft"
	ReportIssued     ReportStatus = "issued"
	ReportSuperseded ReportStatus = "superseded"
)

// Errors surfaced by the report workspace.
var (
	ErrVersionConflict = errors.New("report version conflict: reload and retry")
	ErrReasonRequired  = errors.New("a correction reason note is required")
	ErrCannotDelete    = errors.New("superseded reports must remain readable")
)

// Report is a single version of a lab report. Corrections create a new row
// with Version+1 and mark the prior row superseded. The superseded row is
// kept forever so that clinicians can reconstruct the sequence of results
// a patient was given.
//
// Archival is an orthogonal flag: any report (issued or superseded) can be
// archived for storage management, and archived reports remain retrievable
// via full-text search per the prompt requirement. Archive is a one-way
// operation — unarchiving would complicate retention guarantees and is
// intentionally not exposed. Archived reports are excluded from default
// lists but explicitly included in search results.
type Report struct {
	ID           string
	SampleID     string
	Version      int
	Status       ReportStatus
	Title        string
	Narrative    string
	Measurements []Measurement
	AuthorID     string
	ReasonNote   string // only set on versions >= 2
	IssuedAt     time.Time
	SupersededByID string
	ArchivedAt   time.Time
	ArchivedBy   string
	ArchiveNote  string
	// SearchText is a denormalized column populated on save; it feeds the
	// Postgres tsvector index.
	SearchText string
}

// ErrAlreadyArchived is returned when a caller tries to archive a report
// that has already been archived.
var ErrAlreadyArchived = errors.New("report is already archived")

// Archive marks r as archived at the given time with the supplied actor
// and note. The operation is idempotent-per-actor in the sense that a
// repeated archive by the same user is caught and surfaced to the caller
// (the UI can choose to suppress the error), but it does not silently
// overwrite the earlier archive record.
func Archive(r *Report, actor, note string, at time.Time) error {
	if !r.ArchivedAt.IsZero() {
		return ErrAlreadyArchived
	}
	r.ArchivedAt = at
	r.ArchivedBy = actor
	r.ArchiveNote = note
	return nil
}

// IsArchived is a convenience predicate for filters.
func (r *Report) IsArchived() bool { return !r.ArchivedAt.IsZero() }

// BuildSearchText concatenates title + narrative + measurement codes for
// full-text indexing.
func BuildSearchText(title, narrative string, ms []Measurement) string {
	parts := []string{title, narrative}
	for _, m := range ms {
		parts = append(parts, m.TestCode)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// IssueDraft marks a draft as issued at the given time.
func IssueDraft(r *Report, at time.Time) error {
	if r.Status != ReportDraft {
		return errors.New("only drafts can be issued")
	}
	r.Status = ReportIssued
	r.IssuedAt = at
	return nil
}

// Correct produces a new correction version that supersedes the prior one.
// The caller is expected to persist both the prior and new rows atomically.
// Optimistic concurrency is enforced via expectedVersion: if the prior
// version no longer matches, the function returns ErrVersionConflict.
func Correct(prior *Report, expectedVersion int, title, narrative string, ms []Measurement, author, reason string, at time.Time) (Report, error) {
	if prior.Version != expectedVersion {
		return Report{}, ErrVersionConflict
	}
	if prior.Status == ReportSuperseded {
		return Report{}, errors.New("cannot correct a superseded report; correct the latest version instead")
	}
	if strings.TrimSpace(reason) == "" {
		return Report{}, ErrReasonRequired
	}
	next := Report{
		SampleID:     prior.SampleID,
		Version:      prior.Version + 1,
		Status:       ReportIssued,
		Title:        title,
		Narrative:    narrative,
		Measurements: append([]Measurement(nil), ms...),
		AuthorID:     author,
		ReasonNote:   reason,
		IssuedAt:     at,
		SearchText:   BuildSearchText(title, narrative, ms),
	}
	// The prior is superseded; caller must persist the mutation.
	prior.Status = ReportSuperseded
	return next, nil
}

// CanDelete always returns ErrCannotDelete for superseded reports; it is
// provided to signal intent from handlers.
func CanDelete(r *Report) error {
	if r.Status == ReportSuperseded {
		return ErrCannotDelete
	}
	return nil
}
