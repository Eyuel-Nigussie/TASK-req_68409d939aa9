// Package lab implements the laboratory workflow:
//   - Sample lifecycle state machine (sampling → received → in_testing → reported)
//   - Reference-range evaluation (abnormal / normal / critical)
//   - Report versioning with optimistic concurrency and mandatory reason notes
package lab

import (
	"errors"
	"fmt"
	"time"
)

// SampleStatus is the lifecycle stage of a collected sample.
type SampleStatus string

const (
	SampleSampling   SampleStatus = "sampling"
	SampleReceived   SampleStatus = "received"
	SampleInTesting  SampleStatus = "in_testing"
	SampleReported   SampleStatus = "reported"
	SampleRejected   SampleStatus = "rejected" // operator-driven terminal failure
)

// AllSampleStatuses is the set shown in filter dropdowns.
var AllSampleStatuses = []string{
	string(SampleSampling), string(SampleReceived), string(SampleInTesting),
	string(SampleReported), string(SampleRejected),
}

// sampleTransitions documents allowed moves. Rejection is always available
// until reporting, to let technicians invalidate contaminated specimens.
var sampleTransitions = map[SampleStatus]map[SampleStatus]struct{}{
	SampleSampling:  {SampleReceived: {}, SampleRejected: {}},
	SampleReceived:  {SampleInTesting: {}, SampleRejected: {}},
	SampleInTesting: {SampleReported: {}, SampleRejected: {}},
}

// ErrBadSampleTransition is returned for an invalid transition.
var ErrBadSampleTransition = errors.New("sample transition not allowed")

// Sample represents one collected specimen.
type Sample struct {
	ID          string
	OrderID     string
	CustomerID  string
	Status      SampleStatus
	CollectedAt time.Time
	UpdatedAt   time.Time
	TestCodes   []string
	Notes       string
}

// Transition advances the sample. Returns an event record for audit logging.
type SampleEvent struct {
	SampleID string
	At       time.Time
	From     SampleStatus
	To       SampleStatus
	Actor    string
}

// Transition applies a status change. Rejecting a sample is a one-way move.
func (s *Sample) Transition(to SampleStatus, actor string, at time.Time) (SampleEvent, error) {
	allowed, ok := sampleTransitions[s.Status]
	if !ok {
		return SampleEvent{}, fmt.Errorf("%w: terminal %q", ErrBadSampleTransition, s.Status)
	}
	if _, ok := allowed[to]; !ok {
		return SampleEvent{}, fmt.Errorf("%w: %s -> %s", ErrBadSampleTransition, s.Status, to)
	}
	ev := SampleEvent{SampleID: s.ID, At: at, From: s.Status, To: to, Actor: actor}
	s.Status = to
	s.UpdatedAt = at
	return ev, nil
}
