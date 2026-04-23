package claudelog

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/ijiti/witness/internal/costlog"
)

// StatsCache represents the ~/.claude/stats-cache.json file.
type StatsCache struct {
	Version          int              `json:"version"`
	LastComputedDate string           `json:"lastComputedDate"`
	DailyActivity    []DailyActivity  `json:"dailyActivity"`
	DailyModelTokens []DailyTokens    `json:"dailyModelTokens"`
	DailyCost        []DailyCost      `json:"dailyCost,omitempty"`
	ModelUsage       map[string]ModelUsageStats `json:"modelUsage"`
	TotalSessions    int              `json:"totalSessions"`
	TotalMessages    int              `json:"totalMessages"`
	LongestSession   LongestSession   `json:"longestSession"`
	FirstSessionDate string           `json:"firstSessionDate"`
	HourCounts       map[string]int   `json:"hourCounts"`

	// Computed fields (not in JSON).
	TotalCost float64
}

// DailyActivity holds activity counts for one day.
type DailyActivity struct {
	Date          string `json:"date"`
	MessageCount  int    `json:"messageCount"`
	SessionCount  int    `json:"sessionCount"`
	ToolCallCount int    `json:"toolCallCount"`
}

// DailyTokens holds per-model token counts for one day.
type DailyTokens struct {
	Date          string         `json:"date"`
	TokensByModel map[string]int `json:"tokensByModel"`
}

// DailyCost is the total USD cost of one day's activity across all models.
type DailyCost struct {
	Date string  `json:"date"`
	Cost float64 `json:"cost"`
}

// ModelUsageStats holds aggregate token usage for one model.
type ModelUsageStats struct {
	InputTokens              int     `json:"inputTokens"`
	OutputTokens             int     `json:"outputTokens"`
	CacheReadInputTokens     int     `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int     `json:"cacheCreationInputTokens"`
	CostUSD                  float64 `json:"costUSD"`
}

// LongestSession describes the longest session by duration.
type LongestSession struct {
	SessionID    string `json:"sessionId"`
	Duration     int64  `json:"duration"`     // milliseconds
	MessageCount int    `json:"messageCount"`
	Timestamp    string `json:"timestamp"`
}

// ModelCostSummary is a computed model summary for dashboard display.
type ModelCostSummary struct {
	Model       string
	ShortName   string
	InputTokens int
	OutputTokens int
	CacheTokens int
	TotalTokens int
	Cost        float64
	Pct         float64 // percentage of total tokens
}

// ParseStatsCache reads and parses the stats-cache.json file.
func ParseStatsCache(path string) (*StatsCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var sc StatsCache
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parse stats-cache.json: %w", err)
	}

	// Compute costs from model usage + pricing table.
	for model, usage := range sc.ModelUsage {
		cost := costlog.Cost(model, usage.InputTokens, usage.OutputTokens,
			usage.CacheCreationInputTokens, usage.CacheReadInputTokens)
		sc.TotalCost += cost
	}

	return &sc, nil
}

// ModelSummaries returns sorted model usage summaries with costs.
func (sc *StatsCache) ModelSummaries() []ModelCostSummary {
	var summaries []ModelCostSummary
	var totalTokens int

	for model, usage := range sc.ModelUsage {
		cache := usage.CacheReadInputTokens + usage.CacheCreationInputTokens
		total := usage.InputTokens + usage.OutputTokens + cache
		totalTokens += total

		cost := costlog.Cost(model, usage.InputTokens, usage.OutputTokens,
			usage.CacheCreationInputTokens, usage.CacheReadInputTokens)

		summaries = append(summaries, ModelCostSummary{
			Model:        model,
			ShortName:    shortModelName(model),
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			CacheTokens:  cache,
			TotalTokens:  total,
			Cost:         cost,
		})
	}

	// Compute percentages.
	if totalTokens > 0 {
		for i := range summaries {
			summaries[i].Pct = float64(summaries[i].TotalTokens) / float64(totalTokens) * 100
		}
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].TotalTokens > summaries[j].TotalTokens
	})

	return summaries
}

// MaxDailyMessages returns the peak message count across all days.
func (sc *StatsCache) MaxDailyMessages() int {
	max := 0
	for _, d := range sc.DailyActivity {
		if d.MessageCount > max {
			max = d.MessageCount
		}
	}
	return max
}

// MaxDailyToolCalls returns the peak tool call count across all days.
func (sc *StatsCache) MaxDailyToolCalls() int {
	max := 0
	for _, d := range sc.DailyActivity {
		if d.ToolCallCount > max {
			max = d.ToolCallCount
		}
	}
	return max
}

// MaxHourCount returns the peak count across all hours.
func (sc *StatsCache) MaxHourCount() int {
	max := 0
	for _, c := range sc.HourCounts {
		if c > max {
			max = c
		}
	}
	return max
}

// MaxDailyCost returns the peak USD cost across all days.
func (sc *StatsCache) MaxDailyCost() float64 {
	max := 0.0
	for _, d := range sc.DailyCost {
		if d.Cost > max {
			max = d.Cost
		}
	}
	return max
}


// shortModelName strips common prefixes for compact display.
func shortModelName(model string) string {
	switch {
	case len(model) > 7 && model[:7] == "claude-":
		s := model[7:]
		// Strip date suffixes like -20251101
		if len(s) > 9 {
			tail := s[len(s)-9:]
			if tail[0] == '-' && tail[1] >= '0' && tail[1] <= '9' {
				s = s[:len(s)-9]
			}
		}
		return s
	default:
		return model
	}
}
