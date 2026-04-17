package search

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"  Hello World  ": "hello world",
		"\tHELLO\nworld":  "hello world",
		"":                "",
		"Simple":          "simple",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTokens(t *testing.T) {
	got := Tokens("  Jane  Doe  ")
	if len(got) != 2 || got[0] != "jane" || got[1] != "doe" {
		t.Fatalf("Tokens wrong: %v", got)
	}
	if Tokens("") != nil {
		t.Fatal("empty should yield nil")
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"kitten", "sitting", 3},
		{"", "hello", 5},
		{"hello", "", 5},
	}
	for _, c := range cases {
		if got := Levenshtein(c.a, c.b); got != c.want {
			t.Errorf("Levenshtein(%q,%q)=%d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestScore_ExactMatch(t *testing.T) {
	if s := Score("Jane Doe", "jane doe"); s != 1 {
		t.Fatalf("exact match should be 1, got %v", s)
	}
}

func TestScore_PrefixMatch(t *testing.T) {
	s := Score("jane", "jane doe")
	if s < 0.5 {
		t.Fatalf("prefix match should score >=0.5, got %v", s)
	}
}

func TestScore_Typo(t *testing.T) {
	s := Score("jane doo", "jane doe")
	if s < 0.3 {
		t.Fatalf("typo should still match, got %v", s)
	}
}

func TestScore_Unrelated(t *testing.T) {
	s := Score("xylophone", "jane doe")
	if s > 0.1 {
		t.Fatalf("unrelated should score near 0, got %v", s)
	}
}

func TestRank_SortsByScoreThenLabel(t *testing.T) {
	candidates := []Suggestion{
		{ID: "1", Label: "Jane Doe", Kind: "customer"},
		{ID: "2", Label: "John Doe", Kind: "customer"},
		{ID: "3", Label: "Doe, Jane", Kind: "customer"},
		{ID: "4", Label: "Unrelated Person", Kind: "customer"},
	}
	ranked := Rank("jane doe", candidates, 0.2, 3)
	if len(ranked) != 3 {
		t.Fatalf("expected 3, got %d", len(ranked))
	}
	if ranked[0].Label != "Jane Doe" {
		t.Fatalf("exact match should lead: %v", ranked[0])
	}
	// "Unrelated Person" should have been filtered out.
	for _, r := range ranked {
		if r.Label == "Unrelated Person" {
			t.Fatal("noise not filtered")
		}
	}
}

func TestRank_HandlesEmptyQuery(t *testing.T) {
	got := Rank("", []Suggestion{{Label: "Jane"}}, 0.2, 10)
	if len(got) != 0 {
		t.Fatalf("empty query should yield nothing, got %v", got)
	}
}

func TestRank_LimitZeroMeansNoLimit(t *testing.T) {
	candidates := []Suggestion{
		{Label: "alpha"}, {Label: "alpine"}, {Label: "alps"},
	}
	got := Rank("alpha", candidates, 0, 0)
	if len(got) != 3 {
		t.Fatalf("limit=0 should keep all, got %d", len(got))
	}
}
