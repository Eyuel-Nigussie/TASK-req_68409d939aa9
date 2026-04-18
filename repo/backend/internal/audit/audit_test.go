package audit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/store"
)

func TestLogger_AppendsStructuredEntry(t *testing.T) {
	m := store.NewMemory()
	fixed := time.Unix(1_700_000_000, 0)
	l := New(m, func() time.Time { return fixed })

	type payload struct{ Status string }
	before := payload{Status: "placed"}
	after := payload{Status: "picking"}
	wsTime := fixed.Add(-30 * time.Second)
	if err := l.Log(context.Background(), "u1", "ws-1", wsTime, EntityOrder, "o1", "transition", "", before, after); err != nil {
		t.Fatalf("Log: %v", err)
	}
	entries, _ := l.List(context.Background(), "order", "o1", 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.ActorID != "u1" || e.Workstation != "ws-1" {
		t.Fatalf("actor/workstation mismatch: %+v", e)
	}
	// Workstation time must be recorded alongside server time, and they
	// must remain distinct even when close together.
	if e.WorkstationTime.IsZero() {
		t.Fatal("workstation_time missing from audit entry")
	}
	if !e.At.After(e.WorkstationTime) {
		t.Fatalf("expected server At > workstation time: server=%v ws=%v", e.At, e.WorkstationTime)
	}
	var a payload
	if err := json.Unmarshal(e.After, &a); err != nil || a.Status != "picking" {
		t.Fatalf("after not serialized: %v %+v", err, a)
	}
}

func TestLogger_NilSnapshotsAllowed(t *testing.T) {
	m := store.NewMemory()
	l := New(m, nil)
	if err := l.Log(context.Background(), "u1", "ws-1", time.Time{}, EntityUser, "u1", "login", "", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestLogger_ZeroWorkstationTimeOmitted(t *testing.T) {
	m := store.NewMemory()
	fixed := time.Unix(1_700_000_000, 0)
	l := New(m, func() time.Time { return fixed })
	// Missing header -> zero client time; the server must NOT fabricate one.
	if err := l.Log(context.Background(), "u1", "ws", time.Time{}, EntityOrder, "o", "create", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	entries, _ := l.List(context.Background(), "order", "o", 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !entries[0].WorkstationTime.IsZero() {
		t.Fatalf("expected zero workstation time, got %v", entries[0].WorkstationTime)
	}
}

// faultyAudit wraps an in-memory store but fails every AppendAudit so
// we can exercise the Logger's error-handling branch (M4).
type faultyAudit struct {
	inner *store.Memory
	fail  error
}

func (f *faultyAudit) AppendAudit(ctx context.Context, e models.AuditEntry) error {
	return f.fail
}
func (f *faultyAudit) ListAudit(ctx context.Context, entity, entityID string, limit int) ([]models.AuditEntry, error) {
	return f.inner.ListAudit(ctx, entity, entityID, limit)
}

func TestLogger_AppendFailureSurfacesThroughErrorSink(t *testing.T) {
	boom := errors.New("simulated DB outage")
	f := &faultyAudit{inner: store.NewMemory(), fail: boom}
	l := New(f, nil)
	var (
		gotEntity, gotID, gotAction, gotActor string
		gotErr                                error
	)
	l.SetErrorSink(func(entity, entityID, action, actor string, err error) {
		gotEntity, gotID, gotAction, gotActor, gotErr = entity, entityID, action, actor, err
	})
	err := l.Log(context.Background(), "u1", "ws-1", time.Time{}, EntityOrder, "o42", "transition", "", nil, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom error, got %v", err)
	}
	if gotEntity != "order" || gotID != "o42" || gotAction != "transition" || gotActor != "u1" {
		t.Fatalf("error sink did not capture identifiers: %q %q %q %q", gotEntity, gotID, gotAction, gotActor)
	}
	if !errors.Is(gotErr, boom) {
		t.Fatalf("error sink did not capture underlying error: %v", gotErr)
	}
}

func TestLogger_ListLimit(t *testing.T) {
	m := store.NewMemory()
	l := New(m, nil)
	for i := 0; i < 10; i++ {
		_ = l.Log(context.Background(), "u1", "ws", time.Time{}, EntityOrder, "o1", "touch", "", nil, nil)
	}
	got, _ := l.List(context.Background(), "order", "o1", 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 with limit, got %d", len(got))
	}
}
