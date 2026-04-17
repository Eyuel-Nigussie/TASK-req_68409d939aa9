package order

import (
	"testing"
	"time"
)

func TestTransition_TerminalStatusesBlockAll(t *testing.T) {
	for _, st := range []Status{StatusRefunded, StatusReceived, StatusCanceled} {
		o := &Order{Status: st}
		if o.CanTransitionTo(StatusPicking) {
			t.Errorf("%s should not allow any transition", st)
		}
	}
}

func TestTransition_RefundFromReceivedSupplyReason(t *testing.T) {
	o := &Order{ID: "o", Status: StatusReceived, PlacedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0)}
	if _, err := o.Transition(StatusRefunded, "u", "customer returned", "", time.Now()); err != nil {
		t.Fatalf("received->refunded with reason: %v", err)
	}
}

func TestDetectPickingTimeout_AtExactlyThresholdStillFlags(t *testing.T) {
	base := time.Unix(1, 0)
	o := &Order{Status: StatusPicking, UpdatedAt: base}
	// At exactly 30 minutes — the condition is "< PickingTimeout" for not
	// flagging, so at 30m it flags.
	if ex := DetectPickingTimeout(o, base.Add(PickingTimeout)); ex == nil {
		t.Fatal("at threshold should flag")
	}
}

func TestPlanOutOfStock_Descriptions(t *testing.T) {
	p1 := PlanOutOfStock([]string{"A"}, nil)
	if p1.SuggestSplit {
		t.Fatal("fully available must not split")
	}
	p2 := PlanOutOfStock(nil, []string{"B"})
	if p2.SuggestSplit {
		t.Fatal("no available means hold/cancel, not split")
	}
}
