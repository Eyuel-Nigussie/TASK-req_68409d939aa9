package search

import "testing"

// Rank with a large limit > candidate count returns them all; with
// limit smaller than 0 acts like unlimited.
func TestRank_LimitBiggerThanCount(t *testing.T) {
	cs := []Suggestion{{Label: "alpha"}, {Label: "beta"}}
	out := Rank("alpha", cs, 0, 10)
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
}

// Exact-match short-circuit inside the scorer.
func TestScore_ExactAfterNormalize(t *testing.T) {
	if Score("HELLO world", "  hello   world  ") != 1 {
		t.Fatal("normalized equality should score 1")
	}
}

// Empty candidate inside Rank is filtered by Score returning 0.
func TestRank_SkipsEmptyCandidates(t *testing.T) {
	cs := []Suggestion{{Label: ""}, {Label: "needle"}}
	out := Rank("needle", cs, 0.5, 0)
	if len(out) != 1 || out[0].Label != "needle" {
		t.Fatalf("empty labels must not rank: %+v", out)
	}
}
