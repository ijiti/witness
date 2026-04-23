package claudelog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeJSONLLine returns a JSON-encoded line for a RawRecord, panic on error.
func makeJSONLLine(r *RawRecord) []byte {
	b, err := json.Marshal(r)
	if err != nil {
		panic(fmt.Sprintf("marshal record: %v", err))
	}
	return append(b, '\n')
}

// writeJSONLFile creates a JSONL file in dir with the given records.
func writeJSONLFile(t *testing.T, dir, filename string, records []*RawRecord) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", filename, err)
	}
	defer f.Close()
	for _, r := range records {
		if _, err := f.Write(makeJSONLLine(r)); err != nil {
			t.Fatalf("write %s: %v", filename, err)
		}
	}
	return path
}

// buildTestRecords builds a minimal 1-turn session: user → assistant with tokens.
func buildTestRecords(sessionID, model string, ts time.Time, inputTok, outputTok int) []*RawRecord {
	msgID := sessionID + "-msg"
	stopReason := "end_turn"
	return []*RawRecord{
		{
			Type:      TypeUser,
			UUID:      sessionID + "-u1",
			SessionID: sessionID,
			Timestamp: ts,
			CWD:       "/home/user/project",
			Message: &RawMessage{
				Role:    "user",
				Content: mustMarshal("hello"),
			},
		},
		{
			Type:      TypeAssistant,
			UUID:      sessionID + "-a1",
			SessionID: sessionID,
			Timestamp: ts.Add(time.Second),
			Message: &RawMessage{
				Role:       "assistant",
				ID:         msgID,
				Model:      model,
				StopReason: &stopReason,
				Content:    mustMarshal([]ContentBlock{{Type: "text", Text: "hi"}}),
				Usage: &RawUsage{
					InputTokens:  inputTok,
					OutputTokens: outputTok,
				},
			},
		},
		{
			Type:      TypeSystem,
			UUID:      sessionID + "-s1",
			SessionID: sessionID,
			Timestamp: ts.Add(2 * time.Second),
			Subtype:   SubtypeTurnDuration,
			DurationMs: 2000,
			ParentUUID: func() *string { s := sessionID + "-a1"; return &s }(),
		},
	}
}

func TestComputeStatsEmpty(t *testing.T) {
	dir := t.TempDir()
	// No project dirs — should return zero-value stats with no error.
	sc, err := ComputeStats(dir)
	if err != nil {
		t.Fatalf("ComputeStats on empty dir: %v", err)
	}
	if sc == nil {
		t.Fatal("ComputeStats returned nil")
	}
	if sc.TotalSessions != 0 {
		t.Errorf("TotalSessions = %d, want 0", sc.TotalSessions)
	}
	if sc.TotalMessages != 0 {
		t.Errorf("TotalMessages = %d, want 0", sc.TotalMessages)
	}
}

func TestComputeStatsTwoProjects(t *testing.T) {
	dir := t.TempDir()

	// Day 1: UTC 2026-01-10T09:00:00Z  → hour "9"
	day1 := time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC)
	// Day 2: UTC 2026-01-11T14:00:00Z → hour "14"
	day2 := time.Date(2026, 1, 11, 14, 0, 0, 0, time.UTC)

	// Project A — one session on day1, model sonnet.
	projA := filepath.Join(dir, "-home-user-projA")
	if err := os.MkdirAll(projA, 0o755); err != nil {
		t.Fatal(err)
	}
	recA := buildTestRecords("sess-A1", "claude-sonnet-4-6", day1, 1000, 200)
	writeJSONLFile(t, projA, "sess-A1.jsonl", recA)

	// Project B — two sessions: one on day1, one on day2 with a tool call.
	projB := filepath.Join(dir, "-home-user-projB")
	if err := os.MkdirAll(projB, 0o755); err != nil {
		t.Fatal(err)
	}
	recB1 := buildTestRecords("sess-B1", "claude-sonnet-4-6", day1, 500, 100)
	writeJSONLFile(t, projB, "sess-B1.jsonl", recB1)

	// Session B2 on day2 with a tool_use in the assistant content.
	stopReason := "tool_use"
	recB2 := []*RawRecord{
		{
			Type:      TypeUser,
			UUID:      "sess-B2-u1",
			SessionID: "sess-B2",
			Timestamp: day2,
			CWD:       "/home/user/projB",
			Message: &RawMessage{
				Role:    "user",
				Content: mustMarshal("run tests"),
			},
		},
		{
			Type:      TypeAssistant,
			UUID:      "sess-B2-a1",
			SessionID: "sess-B2",
			Timestamp: day2.Add(time.Second),
			Message: &RawMessage{
				Role:  "assistant",
				ID:    "sess-B2-msg",
				Model: "claude-haiku-4-5",
				StopReason: &stopReason,
				Content: mustMarshal([]ContentBlock{
					{Type: "text", Text: "running"},
					{Type: "tool_use", ID: "t1", Name: "Bash", Input: mustMarshal(map[string]string{"command": "go test ./..."})},
				}),
				Usage: &RawUsage{InputTokens: 300, OutputTokens: 50},
			},
		},
	}
	writeJSONLFile(t, projB, "sess-B2.jsonl", recB2)

	sc, err := ComputeStats(dir)
	if err != nil {
		t.Fatalf("ComputeStats: %v", err)
	}

	// 3 sessions total.
	if sc.TotalSessions != 3 {
		t.Errorf("TotalSessions = %d, want 3", sc.TotalSessions)
	}

	// 3 turns total (one per session).
	if sc.TotalMessages != 3 {
		t.Errorf("TotalMessages = %d, want 3", sc.TotalMessages)
	}

	// FirstSessionDate should be day1 date.
	wantFirst := day1.Format("2006-01-02")
	if sc.FirstSessionDate != wantFirst {
		t.Errorf("FirstSessionDate = %q, want %q", sc.FirstSessionDate, wantFirst)
	}

	// Two distinct days.
	if len(sc.DailyActivity) != 2 {
		t.Errorf("len(DailyActivity) = %d, want 2", len(sc.DailyActivity))
	}

	// DailyActivity is sorted by date — day1 first.
	day1Str := day1.Format("2006-01-02")
	day2Str := day2.Format("2006-01-02")
	if sc.DailyActivity[0].Date != day1Str {
		t.Errorf("DailyActivity[0].Date = %q, want %q", sc.DailyActivity[0].Date, day1Str)
	}
	if sc.DailyActivity[1].Date != day2Str {
		t.Errorf("DailyActivity[1].Date = %q, want %q", sc.DailyActivity[1].Date, day2Str)
	}

	// Day1: 2 sessions, 2 turns/messages, 0 tool calls.
	if sc.DailyActivity[0].SessionCount != 2 {
		t.Errorf("DailyActivity[0].SessionCount = %d, want 2", sc.DailyActivity[0].SessionCount)
	}
	if sc.DailyActivity[0].MessageCount != 2 {
		t.Errorf("DailyActivity[0].MessageCount = %d, want 2", sc.DailyActivity[0].MessageCount)
	}
	if sc.DailyActivity[0].ToolCallCount != 0 {
		t.Errorf("DailyActivity[0].ToolCallCount = %d, want 0", sc.DailyActivity[0].ToolCallCount)
	}

	// Day2: 1 session, 1 message, 1 tool call.
	if sc.DailyActivity[1].SessionCount != 1 {
		t.Errorf("DailyActivity[1].SessionCount = %d, want 1", sc.DailyActivity[1].SessionCount)
	}
	if sc.DailyActivity[1].MessageCount != 1 {
		t.Errorf("DailyActivity[1].MessageCount = %d, want 1", sc.DailyActivity[1].MessageCount)
	}
	if sc.DailyActivity[1].ToolCallCount != 1 {
		t.Errorf("DailyActivity[1].ToolCallCount = %d, want 1", sc.DailyActivity[1].ToolCallCount)
	}

	// Two models present.
	if len(sc.ModelUsage) != 2 {
		t.Errorf("len(ModelUsage) = %d, want 2", len(sc.ModelUsage))
	}

	// Sonnet usage: sessions A1 (1000 in, 200 out) + B1 (500 in, 100 out).
	sonnet, ok := sc.ModelUsage["claude-sonnet-4-6"]
	if !ok {
		t.Fatal("ModelUsage missing claude-sonnet-4-6")
	}
	if sonnet.InputTokens != 1500 {
		t.Errorf("sonnet InputTokens = %d, want 1500", sonnet.InputTokens)
	}
	if sonnet.OutputTokens != 300 {
		t.Errorf("sonnet OutputTokens = %d, want 300", sonnet.OutputTokens)
	}

	// Haiku usage: session B2 (300 in, 50 out).
	haiku, ok := sc.ModelUsage["claude-haiku-4-5"]
	if !ok {
		t.Fatal("ModelUsage missing claude-haiku-4-5")
	}
	if haiku.InputTokens != 300 {
		t.Errorf("haiku InputTokens = %d, want 300", haiku.InputTokens)
	}
	if haiku.OutputTokens != 50 {
		t.Errorf("haiku OutputTokens = %d, want 50", haiku.OutputTokens)
	}

	// HourCounts: hour 9 (day1: 2 turns), hour 14 (day2: 1 turn).
	if sc.HourCounts["9"] != 2 {
		t.Errorf("HourCounts[9] = %d, want 2", sc.HourCounts["9"])
	}
	if sc.HourCounts["14"] != 1 {
		t.Errorf("HourCounts[14] = %d, want 1", sc.HourCounts["14"])
	}

	// TotalCost should be positive (all models have non-zero tokens).
	if sc.TotalCost <= 0 {
		t.Errorf("TotalCost = %v, want > 0", sc.TotalCost)
	}

	// CostUSD should be set on each model entry.
	if sonnet.CostUSD <= 0 {
		t.Errorf("sonnet CostUSD = %v, want > 0", sonnet.CostUSD)
	}

	// DailyModelTokens should have 2 entries (one per day).
	if len(sc.DailyModelTokens) != 2 {
		t.Errorf("len(DailyModelTokens) = %d, want 2", len(sc.DailyModelTokens))
	}

	// Sorted by date — day1 first.
	if sc.DailyModelTokens[0].Date != day1Str {
		t.Errorf("DailyModelTokens[0].Date = %q, want %q", sc.DailyModelTokens[0].Date, day1Str)
	}

	// LastComputedDate should be today (UTC).
	today := time.Now().UTC().Format("2006-01-02")
	if sc.LastComputedDate != today {
		t.Errorf("LastComputedDate = %q, want %q", sc.LastComputedDate, today)
	}
}

func TestComputeStatsSkipsTinyFiles(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "-home-user-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a file under 100 bytes — should be skipped.
	if err := os.WriteFile(filepath.Join(projDir, "tiny.jsonl"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	sc, err := ComputeStats(dir)
	if err != nil {
		t.Fatalf("ComputeStats: %v", err)
	}
	if sc.TotalSessions != 0 {
		t.Errorf("TotalSessions = %d, want 0 (tiny file should be skipped)", sc.TotalSessions)
	}
}

func TestComputeStatsLongestSession(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "-home-user-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ts := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

	// Short session: 2s duration.
	short := buildTestRecords("sess-short", "claude-sonnet-4-6", ts, 100, 20)
	writeJSONLFile(t, projDir, "sess-short.jsonl", short)

	// Long session: 5s duration — override the turn_duration system record.
	long := buildTestRecords("sess-long", "claude-sonnet-4-6", ts, 100, 20)
	// Modify the system record to have 5000ms duration.
	long[2].DurationMs = 5000
	writeJSONLFile(t, projDir, "sess-long.jsonl", long)

	sc, err := ComputeStats(dir)
	if err != nil {
		t.Fatalf("ComputeStats: %v", err)
	}

	if sc.LongestSession.SessionID != "sess-long" {
		t.Errorf("LongestSession.SessionID = %q, want %q", sc.LongestSession.SessionID, "sess-long")
	}
	if sc.LongestSession.Duration != 5000 {
		t.Errorf("LongestSession.Duration = %d, want 5000", sc.LongestSession.Duration)
	}
}
