package claudelog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeAuditFile writes the given lines (joined by newline) to dir/filename.
func writeAuditFile(t *testing.T, dir, filename string, lines ...string) {
	t.Helper()
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0600); err != nil {
		t.Fatalf("writeAuditFile %s: %v", filename, err)
	}
}

func TestLoadAudit_Standard(t *testing.T) {
	dir := t.TempDir()
	writeAuditFile(t, dir, "2026-02-20.jsonl",
		`{"timestamp":"2026-02-20T10:00:00Z","tool":"Bash","args":"ls -la","session":"sess-1","result":"ok"}`,
		`{"timestamp":"2026-02-20T10:01:00Z","tool":"Read","args":"CLAUDE.md","session":"sess-1","result":"ok"}`,
		`{"timestamp":"2026-02-20T10:02:00Z","tool":"Bash","args":"pwd","session":"sess-2","result":"ok"}`,
	)

	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	events := LoadAuditForSession(dir, "sess-1", base, base.Add(time.Hour))

	if got, want := len(events), 2; got != want {
		t.Fatalf("len(events) = %d, want %d", got, want)
	}
	for i, ev := range events {
		if got, want := ev.Type, "tool"; got != want {
			t.Errorf("events[%d].Type = %q, want %q", i, got, want)
		}
		if ev.Tool == "" {
			t.Errorf("events[%d].Tool is empty, want non-empty", i)
		}
	}
}

func TestLoadAudit_NonMatching(t *testing.T) {
	dir := t.TempDir()
	writeAuditFile(t, dir, "2026-02-20.jsonl",
		`{"timestamp":"2026-02-20T10:00:00Z","tool":"Bash","args":"ls","session":"sess-other","result":"ok"}`,
	)

	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	events := LoadAuditForSession(dir, "sess-1", base, base.Add(time.Hour))

	if got, want := len(events), 0; got != want {
		t.Errorf("len(events) = %d, want %d", got, want)
	}
}

func TestLoadAudit_Canary(t *testing.T) {
	dir := t.TempDir()
	writeAuditFile(t, dir, "canary-2026-02-20.jsonl",
		`{"timestamp":"2026-02-20T10:00:00Z","session_id":"sess-1","command":"cat /etc/shadow","matched_canary":"/etc/shadow","label":"sensitive-file"}`,
	)

	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	events := LoadAuditForSession(dir, "sess-1", base, base.Add(time.Hour))

	if got, want := len(events), 1; got != want {
		t.Fatalf("len(events) = %d, want %d", got, want)
	}
	ev := events[0]
	if got, want := ev.Type, "canary"; got != want {
		t.Errorf("Type = %q, want %q", got, want)
	}
	if got, want := ev.Severity, "critical"; got != want {
		t.Errorf("Severity = %q, want %q", got, want)
	}
	if !strings.Contains(ev.Summary, "Canary detected") {
		t.Errorf("Summary = %q, want it to contain %q", ev.Summary, "Canary detected")
	}
}

func TestLoadAudit_Sanitizer(t *testing.T) {
	dir := t.TempDir()
	writeAuditFile(t, dir, "content-sanitizer-2026-02-20.jsonl",
		`{"timestamp":"2026-02-20T10:00:00Z","severity":"warning","tool":"Bash","source":"output","detections":"PII detected","session":"sess-1"}`,
	)

	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	events := LoadAuditForSession(dir, "sess-1", base, base.Add(time.Hour))

	if got, want := len(events), 1; got != want {
		t.Fatalf("len(events) = %d, want %d", got, want)
	}
	ev := events[0]
	if got, want := ev.Type, "sanitizer"; got != want {
		t.Errorf("Type = %q, want %q", got, want)
	}
	if got, want := ev.Severity, "warning"; got != want {
		t.Errorf("Severity = %q, want %q", got, want)
	}
}

func TestLoadAudit_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	// No files written — all three file types are absent.

	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	events := LoadAuditForSession(dir, "sess-1", base, base.Add(time.Hour))

	if len(events) != 0 {
		t.Errorf("len(events) = %d, want 0", len(events))
	}
}

func TestLoadAudit_MultiDay(t *testing.T) {
	dir := t.TempDir()
	writeAuditFile(t, dir, "2026-02-19.jsonl",
		`{"timestamp":"2026-02-19T09:00:00Z","tool":"Bash","args":"whoami","session":"sess-1","result":"ok"}`,
	)
	writeAuditFile(t, dir, "2026-02-20.jsonl",
		`{"timestamp":"2026-02-20T10:00:00Z","tool":"Read","args":"README.md","session":"sess-1","result":"ok"}`,
	)

	start := time.Date(2026, 2, 19, 8, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 20, 11, 0, 0, 0, time.UTC)
	events := LoadAuditForSession(dir, "sess-1", start, end)

	if got, want := len(events), 2; got != want {
		t.Errorf("len(events) = %d, want %d", got, want)
	}
}

func TestLoadAudit_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	writeAuditFile(t, dir, "2026-02-20.jsonl",
		`{"timestamp":"2026-02-20T10:00:00Z","tool":"Bash","args":"ls","session":"sess-1","result":"ok"}`,
		`{invalid json`,
	)

	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	events := LoadAuditForSession(dir, "sess-1", base, base.Add(time.Hour))

	if got, want := len(events), 1; got != want {
		t.Errorf("len(events) = %d, want %d (malformed line should be skipped)", got, want)
	}
}

func TestLoadAudit_EmptyInputs(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		auditDir  string
		sessionID string
		startTime time.Time
	}{
		{"empty auditDir", "", "sess-1", base},
		{"empty sessionID", dir, "", base},
		{"zero startTime", dir, "sess-1", time.Time{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			events := LoadAuditForSession(tc.auditDir, tc.sessionID, tc.startTime, base.Add(time.Hour))
			if events != nil {
				t.Errorf("expected nil, got %v", events)
			}
		})
	}
}

func TestLoadAudit_SortOrder(t *testing.T) {
	dir := t.TempDir()
	// Write entries in non-chronological order: T+2, T+0, T+1.
	writeAuditFile(t, dir, "2026-02-20.jsonl",
		`{"timestamp":"2026-02-20T10:02:00Z","tool":"Bash","args":"date","session":"sess-1","result":"ok"}`,
		`{"timestamp":"2026-02-20T10:00:00Z","tool":"Bash","args":"whoami","session":"sess-1","result":"ok"}`,
		`{"timestamp":"2026-02-20T10:01:00Z","tool":"Read","args":"file.txt","session":"sess-1","result":"ok"}`,
	)

	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	events := LoadAuditForSession(dir, "sess-1", base, base.Add(time.Hour))

	if got, want := len(events), 3; got != want {
		t.Fatalf("len(events) = %d, want %d", got, want)
	}
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp.Before(events[i-1].Timestamp) {
			t.Errorf("events[%d].Timestamp (%v) is before events[%d].Timestamp (%v): not sorted ascending",
				i, events[i].Timestamp, i-1, events[i-1].Timestamp)
		}
	}
}

func TestLoadAudit_ArgsTruncation(t *testing.T) {
	dir := t.TempDir()
	// Args longer than 80 characters — should be truncated with "...".
	longArgs := strings.Repeat("x", 100)
	line := `{"timestamp":"2026-02-20T10:00:00Z","tool":"Bash","args":"` + longArgs + `","session":"sess-1","result":"ok"}`
	writeAuditFile(t, dir, "2026-02-20.jsonl", line)

	base := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	events := LoadAuditForSession(dir, "sess-1", base, base.Add(time.Hour))

	if got, want := len(events), 1; got != want {
		t.Fatalf("len(events) = %d, want %d", got, want)
	}
	if !strings.HasSuffix(events[0].Summary, "...") {
		t.Errorf("Summary = %q, want it to end with %q", events[0].Summary, "...")
	}
}
