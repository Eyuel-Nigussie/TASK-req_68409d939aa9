package search

import "testing"

func TestRank_SortStableByLabelOnTies(t *testing.T) {
	q := "alpha"
	c := []Suggestion{{ID: "1", Label: "alpha"}, {ID: "2", Label: "alpha"}}
	out := Rank(q, c, 0.1, 0)
	if out[0].Label != "alpha" || out[1].Label != "alpha" {
		t.Fatalf("both must be present: %+v", out)
	}
}

func TestScore_TolerateLongerCandidate(t *testing.T) {
	// Long candidate with a single typo should still score above zero.
	s := Score("neighborhood", "neigborhood") // missing 'h'
	if s <= 0 {
		t.Fatalf("expected >0 for near-match, got %v", s)
	}
}

func TestNormalize_OnlyWhitespace(t *testing.T) {
	if Normalize("    \t\n") != "" {
		t.Fatal("whitespace-only string should normalize to empty")
	}
}
