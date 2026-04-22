package costlog

import (
	"math"
	"testing"
)

// almostEqual returns true if a and b are within epsilon of each other.
func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// TestCost verifies the pricing calculation for all known model tiers.
func TestCost(t *testing.T) {
	const eps = 1e-9

	tests := []struct {
		name        string
		model       string
		input       int
		output      int
		cacheCreate int
		cacheRead   int
		wantCostUSD float64
	}{
		{
			name:        "opus zero tokens",
			model:       "claude-opus-4-6",
			input:       0,
			output:      0,
			wantCostUSD: 0,
		},
		{
			name:        "opus 1M input only",
			model:       "claude-opus-4-6",
			input:       1_000_000,
			output:      0,
			wantCostUSD: 15.0,
		},
		{
			name:        "opus 1M output only",
			model:       "claude-opus-4-6",
			input:       0,
			output:      1_000_000,
			wantCostUSD: 75.0,
		},
		{
			name:        "sonnet 1M input 1M output",
			model:       "claude-sonnet-4-6",
			input:       1_000_000,
			output:      1_000_000,
			wantCostUSD: 18.0, // 3 + 15
		},
		{
			name:        "haiku 1M input 1M output",
			model:       "claude-haiku-4-5",
			input:       1_000_000,
			output:      1_000_000,
			wantCostUSD: 4.80, // 0.80 + 4
		},
		{
			name:        "sonnet with cache create and read",
			model:       "claude-sonnet-4-6",
			input:       1_000_000,
			output:      0,
			cacheCreate: 1_000_000,
			cacheRead:   1_000_000,
			wantCostUSD: 3 + 3.75 + 0.30,
		},
		{
			name:        "opus with cache write",
			model:       "claude-opus-4-6",
			cacheCreate: 1_000_000,
			wantCostUSD: 18.75,
		},
		{
			name:        "opus with cache read",
			model:       "claude-opus-4-6",
			cacheRead:   1_000_000,
			wantCostUSD: 1.50,
		},
		{
			name:        "small realistic call",
			model:       "claude-sonnet-4-6",
			input:       5_000,
			output:      500,
			wantCostUSD: 5_000.0/1e6*3 + 500.0/1e6*15,
		},
		// Date-suffix stripping
		{
			name:        "opus with date suffix stripped",
			model:       "claude-opus-4-5-20251101",
			input:       1_000_000,
			wantCostUSD: 15.0,
		},
		{
			name:        "haiku with date suffix stripped",
			model:       "claude-haiku-4-5-20251001",
			output:      1_000_000,
			wantCostUSD: 4.0,
		},
		{
			name:        "sonnet versioned date stamp",
			model:       "claude-sonnet-4-6-20250610",
			input:       1_000_000,
			wantCostUSD: 3.0,
		},
		// Unknown model falls back to sonnet
		{
			name:        "unknown model falls back to sonnet",
			model:       "claude-unknown-99-9",
			input:       1_000_000,
			wantCostUSD: 3.0,
		},
		// Previous generation models
		{
			name:        "opus-4-5",
			model:       "claude-opus-4-5",
			input:       1_000_000,
			wantCostUSD: 15.0,
		},
		{
			name:        "sonnet-4-5",
			model:       "claude-sonnet-4-5",
			output:      1_000_000,
			wantCostUSD: 15.0,
		},
		// All four token types simultaneously
		{
			name:        "all four token types sonnet",
			model:       "claude-sonnet-4-6",
			input:       1_000_000,
			output:      1_000_000,
			cacheCreate: 1_000_000,
			cacheRead:   1_000_000,
			wantCostUSD: 3 + 15 + 3.75 + 0.30,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Cost(tc.model, tc.input, tc.output, tc.cacheCreate, tc.cacheRead)
			if !almostEqual(got, tc.wantCostUSD, eps) {
				t.Errorf("Cost(%q, %d, %d, %d, %d) = %v, want %v",
					tc.model, tc.input, tc.output, tc.cacheCreate, tc.cacheRead,
					got, tc.wantCostUSD)
			}
		})
	}
}

// TestFormatCost verifies human-readable cost formatting.
func TestFormatCost(t *testing.T) {
	tests := []struct {
		cost float64
		want string
	}{
		{0, "$0.00"},
		{0.0005, "<$0.001"},
		{0.001, "$0.001"},
		{0.0019, "$0.002"},
		{0.005, "$0.005"},
		{0.0099, "$0.010"},
		{0.01, "$0.01"},
		{0.016, "$0.02"},
		{1.23456, "$1.23"},
		{10.0, "$10.00"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := FormatCost(tc.cost)
			if got != tc.want {
				t.Errorf("FormatCost(%v) = %q, want %q", tc.cost, got, tc.want)
			}
		})
	}
}
