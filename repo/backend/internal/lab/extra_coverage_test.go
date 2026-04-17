package lab

import (
	"testing"
	"time"
)

func TestSample_TerminalStatusRejectsFurther(t *testing.T) {
	s := &Sample{ID: "s", Status: SampleReported}
	if _, err := s.Transition(SampleInTesting, "u", time.Now()); err == nil {
		t.Fatal("expected error transitioning from reported")
	}
}

func TestEvaluateAll_NoRangeSetReturnsNormal(t *testing.T) {
	ms := []Measurement{{TestCode: "UNKNOWN", Value: 999}}
	out := EvaluateAll(ms, NewRangeSet(), "")
	if out[0].Flag != FlagNormal {
		t.Fatalf("no configured range should default to normal, got %s", out[0].Flag)
	}
}

func TestRangeSet_MatchAnyWhenNoDemographicAndNoGeneral(t *testing.T) {
	rs := NewRangeSet()
	low := 1.0
	rs.Add(RefRange{TestCode: "Z", Demographic: "adult", LowNormal: &low})
	r, err := rs.Match("Z", "pediatric")
	if err != nil {
		t.Fatal(err)
	}
	if r.Demographic != "adult" {
		t.Fatalf("expected fallback to any entry, got %+v", r)
	}
}

func TestEvaluate_LowHighCriticalBoundary(t *testing.T) {
	lowCrit := 10.0
	highCrit := 100.0
	r := RefRange{LowCritical: &lowCrit, HighCritical: &highCrit}
	// Exactly equal to critical still qualifies? critical_low is "<" so
	// 10.0 is NOT critical-low; 9.9 is.
	if Evaluate(10.0, r) == FlagCriticalLow {
		t.Fatal("value == low_critical must not be critical low")
	}
	if Evaluate(9.99, r) != FlagCriticalLow {
		t.Fatal("value below low_critical must flag")
	}
	if Evaluate(100.01, r) != FlagCriticalHigh {
		t.Fatal("value above high_critical must flag")
	}
}

func TestArchive_SetsEveryField(t *testing.T) {
	r := &Report{ID: "r", Status: ReportIssued}
	at := time.Unix(1_700_000_000, 0)
	if err := Archive(r, "u1", "retention", at); err != nil {
		t.Fatal(err)
	}
	if r.ArchivedBy != "u1" || r.ArchiveNote != "retention" || !r.ArchivedAt.Equal(at) {
		t.Fatalf("fields not set: %+v", r)
	}
}

func TestIssueDraft_NonDraftRejected(t *testing.T) {
	r := &Report{Status: ReportIssued}
	if err := IssueDraft(r, time.Now()); err == nil {
		t.Fatal("issued draft should reject")
	}
}

func TestCanDelete_IssuedIsOK(t *testing.T) {
	if err := CanDelete(&Report{Status: ReportIssued}); err != nil {
		t.Fatalf("issued should be deletable by policy: %v", err)
	}
}

func TestCorrect_RejectsWhenPriorSuperseded(t *testing.T) {
	prior := &Report{Version: 1, Status: ReportSuperseded}
	if _, err := Correct(prior, 1, "t", "n", nil, "u", "why", time.Now()); err == nil {
		t.Fatal("correction on superseded should fail")
	}
}
