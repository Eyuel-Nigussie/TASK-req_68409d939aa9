package audit

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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
	if err := l.Log(context.Background(), "u1", "ws-1", wsTime, "order", "o1", "transition", "", before, after); err != nil {
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
	if err := l.Log(context.Background(), "u1", "ws-1", time.Time{}, "login", "u1", "login", "", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestLogger_ZeroWorkstationTimeOmitted(t *testing.T) {
	m := store.NewMemory()
	fixed := time.Unix(1_700_000_000, 0)
	l := New(m, func() time.Time { return fixed })
	// Missing header -> zero client time; the server must NOT fabricate one.
	if err := l.Log(context.Background(), "u1", "ws", time.Time{}, "order", "o", "create", "", nil, nil); err != nil {
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

func TestLogger_ListLimit(t *testing.T) {
	m := store.NewMemory()
	l := New(m, nil)
	for i := 0; i < 10; i++ {
		_ = l.Log(context.Background(), "u1", "ws", time.Time{}, "order", "o1", "touch", "", nil, nil)
	}
	got, _ := l.List(context.Background(), "order", "o1", 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 with limit, got %d", len(got))
	}
}
