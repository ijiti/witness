// Package costlog computes the USD cost of a Claude API call from its token
// usage. Pricing is sourced from Anthropic's public pricing page (USD per 1M
// tokens). Unknown models fall back to Sonnet pricing after stripping any
// trailing date suffix.
package costlog

import (
	"fmt"
	"strings"
)

// modelPricing holds per-million-token prices for known Claude models.
type modelPricing struct {
	input      float64 // per 1M input tokens
	output     float64 // per 1M output tokens
	cacheWrite float64 // per 1M cache creation tokens
	cacheRead  float64 // per 1M cache read tokens
}

// knownPricing is the canonical pricing table. When a model is not found here,
// Cost strips trailing date suffixes before falling back to Sonnet pricing.
var knownPricing = map[string]modelPricing{
	// Current generation
	"claude-opus-4-6":   {input: 15, output: 75, cacheWrite: 18.75, cacheRead: 1.50},
	"claude-sonnet-4-6": {input: 3, output: 15, cacheWrite: 3.75, cacheRead: 0.30},
	"claude-haiku-4-5":  {input: 0.80, output: 4, cacheWrite: 1, cacheRead: 0.08},
	// Previous generation
	"claude-opus-4-5":   {input: 15, output: 75, cacheWrite: 18.75, cacheRead: 1.50},
	"claude-sonnet-4-5": {input: 3, output: 15, cacheWrite: 3.75, cacheRead: 0.30},
	// Versioned date-stamped aliases (kept for completeness; Cost also strips suffixes)
	"claude-sonnet-4-20250514":   {input: 3, output: 15, cacheWrite: 3.75, cacheRead: 0.30},
	"claude-opus-4-20250514":     {input: 15, output: 75, cacheWrite: 18.75, cacheRead: 1.50},
	"claude-opus-4-5-20251101":   {input: 15, output: 75, cacheWrite: 18.75, cacheRead: 1.50},
	"claude-haiku-4-5-20251001":  {input: 0.80, output: 4, cacheWrite: 1, cacheRead: 0.08},
	"claude-sonnet-4-6-20250610": {input: 3, output: 15, cacheWrite: 3.75, cacheRead: 0.30},
}

// Cost calculates the USD cost for a given model and token usage.
// If the model is unknown, it attempts to strip a trailing 8-digit date suffix
// (e.g., "claude-opus-4-5-20251101" → "claude-opus-4-5") and look up the
// result. Unknown models after stripping fall back to Sonnet pricing.
func Cost(model string, inputTokens, outputTokens, cacheCreate, cacheRead int) float64 {
	p, ok := knownPricing[model]
	if !ok {
		// Strip trailing date suffix, e.g. "-20251001" (8 digits).
		parts := strings.Split(model, "-")
		if len(parts) >= 4 {
			last := parts[len(parts)-1]
			if len(last) == 8 && last[0] >= '0' && last[0] <= '9' {
				short := strings.Join(parts[:len(parts)-1], "-")
				p, ok = knownPricing[short]
			}
		}
		if !ok {
			// Fallback: sonnet pricing
			p = knownPricing["claude-sonnet-4-6"]
		}
	}
	return float64(inputTokens)/1e6*p.input +
		float64(outputTokens)/1e6*p.output +
		float64(cacheCreate)/1e6*p.cacheWrite +
		float64(cacheRead)/1e6*p.cacheRead
}

// FormatCost formats a USD cost for human-readable display.
func FormatCost(cost float64) string {
	if cost == 0 {
		return "$0.00"
	}
	if cost < 0.001 {
		return "<$0.001"
	}
	if cost < 0.01 {
		return fmt.Sprintf("$%.3f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}
