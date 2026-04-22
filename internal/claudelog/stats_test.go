package claudelog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeStatsFixture writes a stats-cache JSON fixture to a temp file and returns the path.
func writeStatsFixture(t *testing.T, sc StatsCache) string {
	t.Helper()
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal stats fixture: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "stats-cache.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write stats fixture: %v", err)
	}
	return path
}

func TestParseStatsCache(t *testing.T) {
	fixture := StatsCache{
		Version:          1,
		LastComputedDate: "2026-02-20",
		TotalSessions:    42,
		TotalMessages:    1337,
		DailyActivity: []DailyActivity{
			{Date: "2026-02-19", MessageCount: 50, SessionCount: 3, ToolCallCount: 120},
			{Date: "2026-02-20", MessageCount: 75, SessionCount: 5, ToolCallCount: 200},
		},
		ModelUsage: map[string]ModelUsageStats{
			"claude-opus-4-6": {
				InputTokens:              100_000,
				OutputTokens:             10_000,
				CacheReadInputTokens:     5_000,
				CacheCreationInputTokens: 2_000,
			},
		},
		HourCounts: map[string]int{
			"09": 10,
			"14": 25,
			"22": 5,
		},
		FirstSessionDate: "2025-01-01",
		LongestSession: LongestSession{
			SessionID:    "abc-123",
			Duration:     3600000, // 1 hour in ms
			MessageCount: 50,
			Timestamp:    "2026-01-15T10:00:00Z",
		},
	}

	path := writeStatsFixture(t, fixture)

	sc, err := ParseStatsCache(path)
	if err != nil {
		t.Fatalf("ParseStatsCache() error: %v", err)
	}

	if sc.Version != 1 {
		t.Errorf("Version = %d, want 1", sc.Version)
	}
	if sc.LastComputedDate != "2026-02-20" {
		t.Errorf("LastComputedDate = %q, want %q", sc.LastComputedDate, "2026-02-20")
	}
	if sc.TotalSessions != 42 {
		t.Errorf("TotalSessions = %d, want 42", sc.TotalSessions)
	}
	if sc.TotalMessages != 1337 {
		t.Errorf("TotalMessages = %d, want 1337", sc.TotalMessages)
	}
	if len(sc.DailyActivity) != 2 {
		t.Errorf("len(DailyActivity) = %d, want 2", len(sc.DailyActivity))
	}
	if sc.DailyActivity[0].MessageCount != 50 {
		t.Errorf("DailyActivity[0].MessageCount = %d, want 50", sc.DailyActivity[0].MessageCount)
	}
	if _, ok := sc.ModelUsage["claude-opus-4-6"]; !ok {
		t.Error("ModelUsage missing claude-opus-4-6")
	}
	if sc.LongestSession.SessionID != "abc-123" {
		t.Errorf("LongestSession.SessionID = %q, want %q", sc.LongestSession.SessionID, "abc-123")
	}
	if len(sc.HourCounts) != 3 {
		t.Errorf("len(HourCounts) = %d, want 3", len(sc.HourCounts))
	}

	// TotalCost should be computed (non-zero given opus tokens).
	if sc.TotalCost <= 0 {
		t.Errorf("TotalCost = %v, want > 0 (should be computed from ModelUsage)", sc.TotalCost)
	}
}

func TestParseStatsCacheFileNotFound(t *testing.T) {
	_, err := ParseStatsCache("/nonexistent/path/stats-cache.json")
	if err == nil {
		t.Error("ParseStatsCache() expected error for missing file, got nil")
	}
}

func TestModelSummaries(t *testing.T) {
	sc := &StatsCache{
		ModelUsage: map[string]ModelUsageStats{
			"claude-opus-4-6": {
				InputTokens:  500_000,
				OutputTokens: 100_000,
			},
			"claude-haiku-4-5": {
				InputTokens:  200_000,
				OutputTokens: 50_000,
			},
			"claude-sonnet-4-6": {
				InputTokens:  1_000_000,
				OutputTokens: 200_000,
			},
		},
	}

	summaries := sc.ModelSummaries()

	// Should return one entry per model.
	if len(summaries) != 3 {
		t.Fatalf("len(ModelSummaries()) = %d, want 3", len(summaries))
	}

	// Should be sorted by TotalTokens descending.
	for i := 1; i < len(summaries); i++ {
		if summaries[i].TotalTokens > summaries[i-1].TotalTokens {
			t.Errorf("summaries not sorted by TotalTokens desc: [%d]=%d > [%d]=%d",
				i, summaries[i].TotalTokens, i-1, summaries[i-1].TotalTokens)
		}
	}

	// Percentages should sum to ~100.
	var totalPct float64
	for _, s := range summaries {
		totalPct += s.Pct
		if s.Pct <= 0 || s.Pct > 100 {
			t.Errorf("model %q has invalid Pct=%v (want 0 < pct <= 100)", s.Model, s.Pct)
		}
	}
	if totalPct < 99.9 || totalPct > 100.1 {
		t.Errorf("sum of Pct = %v, want ~100", totalPct)
	}

	// Cost should be positive.
	for _, s := range summaries {
		if s.Cost <= 0 {
			t.Errorf("model %q has Cost=%v, want > 0", s.Model, s.Cost)
		}
	}
}

func TestModelSummariesEmpty(t *testing.T) {
	sc := &StatsCache{}
	summaries := sc.ModelSummaries()
	if summaries != nil {
		t.Errorf("ModelSummaries() on empty cache = %v, want nil", summaries)
	}
}

func TestShortModelName(t *testing.T) {
	// shortModelName is unexported, but we can test it via ModelSummaries output.
	tests := []struct {
		model     string
		wantShort string
	}{
		{"claude-opus-4-6", "opus-4-6"},
		{"claude-sonnet-4-6", "sonnet-4-6"},
		{"claude-haiku-4-5", "haiku-4-5"},
		{"claude-haiku-4-5-20251001", "haiku-4-5"},
		{"claude-opus-4-5-20251101", "opus-4-5"},
		{"claude-sonnet-4-6-20250610", "sonnet-4-6"},
	}

	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			sc := &StatsCache{
				ModelUsage: map[string]ModelUsageStats{
					tc.model: {InputTokens: 1000, OutputTokens: 100},
				},
			}
			summaries := sc.ModelSummaries()
			if len(summaries) != 1 {
				t.Fatalf("expected 1 summary, got %d", len(summaries))
			}
			if summaries[0].ShortName != tc.wantShort {
				t.Errorf("ShortName for %q = %q, want %q", tc.model, summaries[0].ShortName, tc.wantShort)
			}
		})
	}
}

func TestMaxDailyMessages(t *testing.T) {
	sc := &StatsCache{
		DailyActivity: []DailyActivity{
			{Date: "2026-01-01", MessageCount: 10},
			{Date: "2026-01-02", MessageCount: 50},
			{Date: "2026-01-03", MessageCount: 25},
		},
	}
	if got := sc.MaxDailyMessages(); got != 50 {
		t.Errorf("MaxDailyMessages() = %d, want 50", got)
	}
}

func TestMaxDailyMessagesEmpty(t *testing.T) {
	sc := &StatsCache{}
	if got := sc.MaxDailyMessages(); got != 0 {
		t.Errorf("MaxDailyMessages() on empty = %d, want 0", got)
	}
}

func TestMaxHourCount(t *testing.T) {
	sc := &StatsCache{
		HourCounts: map[string]int{
			"09": 15,
			"14": 42,
			"20": 8,
		},
	}
	if got := sc.MaxHourCount(); got != 42 {
		t.Errorf("MaxHourCount() = %d, want 42", got)
	}
}

func TestMaxHourCountEmpty(t *testing.T) {
	sc := &StatsCache{}
	if got := sc.MaxHourCount(); got != 0 {
		t.Errorf("MaxHourCount() on empty = %d, want 0", got)
	}
}
