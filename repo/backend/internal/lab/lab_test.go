package lab

import (
	"errors"
	"testing"
	"time"
)

func ptr(f float64) *float64 { return &f }

// ---------- Sample lifecycle ----------

func TestSample_HappyLifecycle(t *testing.T) {
	s := &Sample{ID: "s1", Status: SampleSampling, CollectedAt: time.Unix(1_700_000_000, 0)}
	for _, to := range []SampleStatus{SampleReceived, SampleInTesting, SampleReported} {
		if _, err := s.Transition(to, "u1", time.Now()); err != nil {
			t.Fatalf("move to %s: %v", to, err)
		}
	}
	if s.Status != SampleReported {
		t.Fatalf("final status: %s", s.Status)
	}
}

func TestSample_BackwardsBlocked(t *testing.T) {
	s := &Sample{ID: "s1", Status: SampleReceived}
	if _, err := s.Transition(SampleSampling, "u1", time.Now()); !errors.Is(err, ErrBadSampleTransition) {
		t.Fatalf("backwards should fail, got %v", err)
	}
}

func TestSample_RejectedTerminal(t *testing.T) {
	s := &Sample{ID: "s1", Status: SampleReceived}
	if _, err := s.Transition(SampleRejected, "u1", time.Now()); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, err := s.Transition(SampleInTesting, "u1", time.Now()); !errors.Is(err, ErrBadSampleTransition) {
		t.Fatalf("post-reject transition should fail, got %v", err)
	}
}

// ---------- Reference ranges ----------

func TestEvaluate_AllFlags(t *testing.T) {
	r := RefRange{
		TestCode:     "GLU",
		LowNormal:    ptr(70),
		HighNormal:   ptr(99),
		LowCritical:  ptr(40),
		HighCritical: ptr(400),
	}
	cases := []struct {
		value float64
		want  Flag
	}{
		{85, FlagNormal},
		{60, FlagLow},
		{130, FlagHigh},
		{30, FlagCriticalLow},
		{500, FlagCriticalHigh},
	}
	for _, c := range cases {
		if got := Evaluate(c.value, r); got != c.want {
			t.Errorf("Evaluate(%v) = %s, want %s", c.value, got, c.want)
		}
	}
}

func TestFlag_IsAbnormal(t *testing.T) {
	if FlagNormal.IsAbnormal() {
		t.Error("normal should not be abnormal")
	}
	if !FlagCriticalHigh.IsAbnormal() {
		t.Error("critical_high should be abnormal")
	}
	if FlagUnmeasurable.IsAbnormal() {
		t.Error("unmeasurable is not colored abnormal")
	}
}

func TestRangeSet_Match(t *testing.T) {
	rs := NewRangeSet()
	rs.Add(RefRange{TestCode: "GLU", Demographic: "pediatric", LowNormal: ptr(60), HighNormal: ptr(100)})
	rs.Add(RefRange{TestCode: "GLU", Demographic: "", LowNormal: ptr(70), HighNormal: ptr(99)})
	r, err := rs.Match("GLU", "pediatric")
	if err != nil || *r.LowNormal != 60 {
		t.Fatalf("pediatric match wrong: %+v", r)
	}
	r, _ = rs.Match("GLU", "adult")
	if *r.LowNormal != 70 {
		t.Fatalf("adult should fall back to general: %+v", r)
	}
	if _, err := rs.Match("UNKNOWN", ""); err != ErrNoRefRange {
		t.Fatalf("expected ErrNoRefRange, got %v", err)
	}
}

func TestEvaluateAll(t *testing.T) {
	rs := NewRangeSet()
	rs.Add(RefRange{TestCode: "GLU", LowNormal: ptr(70), HighNormal: ptr(99)})
	ms := []Measurement{
		{TestCode: "GLU", Value: 85},
		{TestCode: "GLU", Value: 200},
		{TestCode: "LIP", Value: 50},               // no range
		{TestCode: "GLU", Unmeasurable: true},
	}
	out := EvaluateAll(ms, rs, "")
	if out[0].Flag != FlagNormal {
		t.Fatal("first should be normal")
	}
	if out[1].Flag != FlagHigh {
		t.Fatal("second should be high")
	}
	if out[2].Flag != FlagNormal {
		t.Fatal("no range -> normal fallback")
	}
	if out[3].Flag != FlagUnmeasurable {
		t.Fatal("unmeasurable should be flagged")
	}
}

// ---------- Report versioning ----------

func makeIssued() *Report {
	return &Report{
		ID: "r1", SampleID: "s1", Version: 1, Status: ReportIssued,
		Title: "CBC", Narrative: "initial narrative",
	}
}

func TestCorrect_CreatesV2AndSupersedes(t *testing.T) {
	orig := makeIssued()
	next, err := Correct(orig, 1, "CBC", "updated", nil, "u1", "typo in narrative", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if next.Version != 2 || next.Status != ReportIssued {
		t.Fatalf("new version wrong: %+v", next)
	}
	if orig.Status != ReportSuperseded {
		t.Fatalf("prior should be superseded: %+v", orig)
	}
}

func TestCorrect_RejectsStaleVersion(t *testing.T) {
	orig := makeIssued()
	if _, err := Correct(orig, 99, "", "", nil, "u1", "reason", time.Now()); err != ErrVersionConflict {
		t.Fatalf("expected ErrVersionConflict, got %v", err)
	}
}

func TestCorrect_RejectsBlankReason(t *testing.T) {
	orig := makeIssued()
	if _, err := Correct(orig, 1, "", "", nil, "u1", "  ", time.Now()); err != ErrReasonRequired {
		t.Fatalf("expected ErrReasonRequired, got %v", err)
	}
}

func TestCorrect_RejectsSupersededBase(t *testing.T) {
	orig := makeIssued()
	orig.Status = ReportSuperseded
	if _, err := Correct(orig, 1, "", "", nil, "u1", "reason", time.Now()); err == nil {
		t.Fatal("expected error when basing correction on superseded row")
	}
}

func TestIssueDraft(t *testing.T) {
	r := &Report{Status: ReportDraft}
	if err := IssueDraft(r, time.Unix(1_700_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	if r.Status != ReportIssued {
		t.Fatal("status should be issued")
	}
	if err := IssueDraft(r, time.Now()); err == nil {
		t.Fatal("issuing non-draft should fail")
	}
}

func TestCanDelete(t *testing.T) {
	if err := CanDelete(&Report{Status: ReportSuperseded}); err != ErrCannotDelete {
		t.Fatalf("expected ErrCannotDelete, got %v", err)
	}
	if err := CanDelete(&Report{Status: ReportIssued}); err != nil {
		t.Fatalf("issued should be deletable per policy: %v", err)
	}
}

func TestArchive_OneWay(t *testing.T) {
	r := &Report{ID: "r1", Status: ReportIssued, Title: "X"}
	at := time.Unix(1_700_000_000, 0)
	if err := Archive(r, "u1", "retention", at); err != nil {
		t.Fatal(err)
	}
	if !r.IsArchived() || r.ArchivedBy != "u1" || r.ArchiveNote != "retention" {
		t.Fatalf("state wrong: %+v", r)
	}
	if err := Archive(r, "u2", "again", at.Add(time.Second)); err != ErrAlreadyArchived {
		t.Fatalf("expected ErrAlreadyArchived, got %v", err)
	}
}

func TestBuildSearchText(t *testing.T) {
	txt := BuildSearchText("CBC", "notes here", []Measurement{{TestCode: "GLU"}, {TestCode: "LIP"}})
	if txt != "cbc notes here glu lip" {
		t.Fatalf("search text wrong: %q", txt)
	}
}
