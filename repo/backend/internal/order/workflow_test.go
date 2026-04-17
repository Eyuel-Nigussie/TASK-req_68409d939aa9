package order

import (
	"errors"
	"testing"
	"time"
)

func makeOrder() *Order {
	return &Order{
		ID:        "o1",
		Status:    StatusPlaced,
		PlacedAt:  time.Unix(1_700_000_000, 0),
		UpdatedAt: time.Unix(1_700_000_000, 0),
	}
}

func TestTransition_HappyPath(t *testing.T) {
	o := makeOrder()
	steps := []Status{StatusPicking, StatusDispatched, StatusDelivered, StatusReceived}
	for i, s := range steps {
		_, err := o.Transition(s, "u1", "", "", time.Unix(1_700_000_000+int64(i*60), 0))
		if err != nil {
			t.Fatalf("step %d (%s) failed: %v", i, s, err)
		}
	}
	if o.Status != StatusReceived || len(o.Events) != 4 {
		t.Fatalf("unexpected final state: %+v", o)
	}
}

func TestTransition_Forbidden(t *testing.T) {
	o := makeOrder()
	if _, err := o.Transition(StatusDelivered, "u1", "", "", time.Now()); !errors.Is(err, ErrBadTransition) {
		t.Fatalf("expected ErrBadTransition, got %v", err)
	}
}

func TestTransition_RefundRequiresReason(t *testing.T) {
	o := makeOrder()
	_, _ = o.Transition(StatusPicking, "u1", "", "", time.Now())
	_, err := o.Transition(StatusRefunded, "u1", "", "", time.Now())
	if err != ErrNoReason {
		t.Fatalf("expected ErrNoReason, got %v", err)
	}
	_, err = o.Transition(StatusRefunded, "u1", "customer requested", "", time.Now())
	if err != nil {
		t.Fatalf("reason should be accepted: %v", err)
	}
}

func TestTransition_TerminalBlocks(t *testing.T) {
	o := makeOrder()
	_, _ = o.Transition(StatusCanceled, "u1", "", "", time.Now())
	if _, err := o.Transition(StatusPicking, "u1", "", "", time.Now()); !errors.Is(err, ErrBadTransition) {
		t.Fatalf("terminal status should reject, got %v", err)
	}
}

func TestCanTransitionTo(t *testing.T) {
	o := makeOrder()
	if !o.CanTransitionTo(StatusPicking) {
		t.Fatal("placed->picking should be allowed")
	}
	if o.CanTransitionTo(StatusReceived) {
		t.Fatal("placed->received should be blocked")
	}
}

func TestDetectPickingTimeout(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	o := makeOrder()
	_, _ = o.Transition(StatusPicking, "u1", "", "", base)
	if ex := DetectPickingTimeout(o, base.Add(29*time.Minute)); ex != nil {
		t.Fatalf("within window should not flag: %+v", ex)
	}
	if ex := DetectPickingTimeout(o, base.Add(31*time.Minute)); ex == nil {
		t.Fatal("after threshold must flag")
	}
	// Non-picking status: no timeout.
	o.Status = StatusDispatched
	if ex := DetectPickingTimeout(o, base.Add(time.Hour)); ex != nil {
		t.Fatalf("non-picking should not flag: %+v", ex)
	}
}

func TestDetectOutOfStock(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	o := &Order{ID: "o1", Items: []LineItem{{SKU: "A"}, {SKU: "B", Backordered: true}}}
	ex := DetectOutOfStock(o, base)
	if ex == nil || ex.Kind != "out_of_stock" {
		t.Fatalf("expected OOS, got %+v", ex)
	}
	// No backordered items -> no exception.
	o2 := &Order{ID: "o2", Items: []LineItem{{SKU: "A"}}}
	if ex := DetectOutOfStock(o2, base); ex != nil {
		t.Fatalf("expected nil, got %+v", ex)
	}
	// Nil / empty -> nil.
	if DetectOutOfStock(nil, base) != nil || DetectOutOfStock(&Order{}, base) != nil {
		t.Fatal("nil/empty input must yield nil")
	}
}

func TestMarkItemBackordered(t *testing.T) {
	o := &Order{Items: []LineItem{{SKU: "A", Backordered: false}}}
	if changed := o.MarkItemBackordered("A", true); !changed {
		t.Fatal("expected change")
	}
	if !o.Items[0].Backordered {
		t.Fatal("item should be backordered")
	}
	// Same state is a no-op.
	if changed := o.MarkItemBackordered("A", true); changed {
		t.Fatal("expected no-op for same state")
	}
	// Unknown SKU -> no change.
	if changed := o.MarkItemBackordered("Z", true); changed {
		t.Fatal("unknown SKU must be a no-op")
	}
}

func TestPlanOutOfStock(t *testing.T) {
	p := PlanOutOfStock([]string{"A"}, []string{"B"})
	if !p.SuggestSplit {
		t.Fatal("expected split")
	}
	p = PlanOutOfStock([]string{"A", "B"}, nil)
	if p.SuggestSplit {
		t.Fatal("no backorder should not split")
	}
	p = PlanOutOfStock(nil, []string{"A"})
	if p.SuggestSplit {
		t.Fatal("nothing available should not split")
	}
}
