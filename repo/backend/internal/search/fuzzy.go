// Package search provides typo-tolerant client-side ranking used by the
// global search bar. For large datasets the database does the heavy lifting
// via full-text indexes; this package refines a smaller candidate set
// (typically <500 rows) returned from Postgres and scores them for display.
package search

import (
	"sort"
	"strings"
	"unicode"
)

// Normalize lowercases the input and collapses runs of whitespace. This
// matches the normalization that Postgres full-text search applies so that
// scores are comparable across layers.
func Normalize(s string) string {
	var b strings.Builder
	prevSpace := true // start with "prevSpace" so we don't emit leading space
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			if !prevSpace {
				b.WriteByte(' ')
			}
			prevSpace = true
		default:
			b.WriteRune(unicode.ToLower(r))
			prevSpace = false
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// Tokens splits a normalized string into whitespace-separated tokens.
func Tokens(s string) []string {
	n := Normalize(s)
	if n == "" {
		return nil
	}
	return strings.Split(n, " ")
}

// Levenshtein returns the edit distance between a and b. Runs in O(n*m) time
// which is acceptable for the short strings used in global search queries.
func Levenshtein(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// Score returns a ranking score in [0,1] for a query against a candidate.
// A score of 1 is a perfect match; 0 is no overlap. The scoring combines:
//   - Exact substring match (strong signal): +0.6
//   - Prefix match (common typing behavior):  +0.3
//   - Token-wise best fuzzy overlap:          variable
//
// Typos up to roughly 20% of the token length are tolerated, which matches
// product expectation for operators under time pressure.
func Score(query, candidate string) float64 {
	q := Normalize(query)
	c := Normalize(candidate)
	if q == "" || c == "" {
		return 0
	}
	if q == c {
		return 1
	}
	var score float64
	if strings.Contains(c, q) {
		score += 0.6
	}
	if strings.HasPrefix(c, q) {
		score += 0.3
	}
	qTokens := strings.Split(q, " ")
	cTokens := strings.Split(c, " ")
	var tokenScore float64
	for _, qt := range qTokens {
		best := 0.0
		for _, ct := range cTokens {
			if qt == ct {
				best = 1
				break
			}
			d := Levenshtein(qt, ct)
			// Tolerance: allow up to max(1, len/5) edits.
			maxLen := len(qt)
			if len(ct) > maxLen {
				maxLen = len(ct)
			}
			tol := maxLen / 5
			if tol < 1 {
				tol = 1
			}
			if d <= tol {
				s := 1 - float64(d)/float64(maxLen+1)
				if s > best {
					best = s
				}
			}
		}
		tokenScore += best
	}
	tokenScore /= float64(len(qTokens))
	score += tokenScore * 0.4
	if score > 1 {
		score = 1
	}
	return score
}

// Suggestion is a ranked hit for display in the global search dropdown.
type Suggestion struct {
	ID    string
	Label string
	Kind  string  // "customer", "order", "report", etc.
	Score float64
}

// Rank scores and sorts candidates by descending score, filters out noise
// (score < minScore), and caps the result to `limit`.
func Rank(query string, candidates []Suggestion, minScore float64, limit int) []Suggestion {
	scored := make([]Suggestion, 0, len(candidates))
	for _, c := range candidates {
		c.Score = Score(query, c.Label)
		if c.Score >= minScore {
			scored = append(scored, c)
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		// Stable tie-break by Label for predictable UI order.
		return scored[i].Label < scored[j].Label
	})
	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}
	return scored
}
