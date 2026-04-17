package lab

import "errors"

// Flag is the abnormality verdict for a single measurement.
type Flag string

const (
	FlagNormal         Flag = "normal"
	FlagLow            Flag = "low"
	FlagHigh           Flag = "high"
	FlagCriticalLow    Flag = "critical_low"
	FlagCriticalHigh   Flag = "critical_high"
	FlagUnmeasurable   Flag = "unmeasurable"
	// FlagUncategorized is assigned when no reference range is
	// configured for the test code. It is distinct from FlagNormal so
	// the UI can render the row neutrally rather than green, which would
	// otherwise misrepresent an uncategorized test as clinically normal.
	FlagUncategorized  Flag = "uncategorized"
)

// IsAbnormal is true for results that should be highlighted. Normal,
// unmeasurable, and uncategorized readings are NOT abnormal — the
// latter because we have no reference range to compare against.
func (f Flag) IsAbnormal() bool {
	return f != FlagNormal && f != FlagUnmeasurable && f != FlagUncategorized
}

// IsCritical is true when the result falls outside the critical bounds.
func (f Flag) IsCritical() bool { return f == FlagCriticalLow || f == FlagCriticalHigh }

// RefRange describes the normal and optional critical bounds for a test.
// Bounds are inclusive. A nil pointer means "no boundary on that side".
type RefRange struct {
	TestCode    string
	Units       string
	LowNormal   *float64
	HighNormal  *float64
	LowCritical *float64
	HighCritical *float64
	// Demographics can be used for age/sex-specific ranges; left empty for
	// a general adult range.
	Demographic string
}

// ErrNoRefRange is returned when no matching range exists for a test code.
var ErrNoRefRange = errors.New("no reference range configured")

// RangeSet is an indexed collection of RefRange values.
type RangeSet struct {
	byCode map[string][]RefRange
}

// NewRangeSet constructs an empty collection.
func NewRangeSet() *RangeSet {
	return &RangeSet{byCode: make(map[string][]RefRange)}
}

// Add appends a range to the set. Multiple ranges may exist per test code
// when they target different demographics; Match picks the first entry whose
// demographic matches or is empty.
func (rs *RangeSet) Add(r RefRange) { rs.byCode[r.TestCode] = append(rs.byCode[r.TestCode], r) }

// Match returns the best matching range for a test code and demographic, or
// ErrNoRefRange. An empty demographic in the input means "general".
func (rs *RangeSet) Match(testCode, demo string) (RefRange, error) {
	list, ok := rs.byCode[testCode]
	if !ok || len(list) == 0 {
		return RefRange{}, ErrNoRefRange
	}
	for _, r := range list {
		if r.Demographic == demo {
			return r, nil
		}
	}
	for _, r := range list {
		if r.Demographic == "" {
			return r, nil
		}
	}
	return list[0], nil
}

// Evaluate categorizes a numeric result against a reference range.
// The ordering of checks ensures "critical" wins over "high/low" when both
// could apply.
func Evaluate(value float64, r RefRange) Flag {
	if r.LowCritical != nil && value < *r.LowCritical {
		return FlagCriticalLow
	}
	if r.HighCritical != nil && value > *r.HighCritical {
		return FlagCriticalHigh
	}
	if r.LowNormal != nil && value < *r.LowNormal {
		return FlagLow
	}
	if r.HighNormal != nil && value > *r.HighNormal {
		return FlagHigh
	}
	return FlagNormal
}

// Measurement captures a single recorded test result.
type Measurement struct {
	TestCode string
	Value    float64
	Units    string
	// Unmeasurable marks a test that could not produce a numeric value
	// (e.g., sample lysed, below limit of detection); Flag() returns
	// FlagUnmeasurable regardless of the stored value.
	Unmeasurable bool
	Flag         Flag
}

// EvaluateAll computes flags for each measurement against the range set.
// Returns a slice in the same order as the input so the caller can diff.
func EvaluateAll(ms []Measurement, rs *RangeSet, demo string) []Measurement {
	out := make([]Measurement, len(ms))
	for i, m := range ms {
		if m.Unmeasurable {
			m.Flag = FlagUnmeasurable
			out[i] = m
			continue
		}
		r, err := rs.Match(m.TestCode, demo)
		if err != nil {
			// No range configured for this test code — mark neutrally so
			// the UI doesn't advertise an uncategorized result as normal.
			m.Flag = FlagUncategorized
			out[i] = m
			continue
		}
		m.Flag = Evaluate(m.Value, r)
		out[i] = m
	}
	return out
}
