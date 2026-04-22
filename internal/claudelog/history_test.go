package claudelog

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// writeHistoryFixture marshals each entry as a JSON line and writes to a temp file.
// Returns the path of the temp file. The file is cleaned up by t.Cleanup.
func writeHistoryFixture(t *testing.T, entries []HistoryEntry) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "history-*.jsonl")
	if err != nil {
		t.Fatalf("writeHistoryFixture: create temp file: %v", err)
	}
	defer f.Close()

	for _, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("writeHistoryFixture: marshal entry: %v", err)
		}
		if _, err := fmt.Fprintf(f, "%s\n", b); err != nil {
			t.Fatalf("writeHistoryFixture: write entry: %v", err)
		}
	}
	return f.Name()
}

func TestParseHistoryFile_Basic(t *testing.T) {
	entries := []HistoryEntry{
		{Display: "first prompt", Timestamp: 1000, Project: "/home/user/proj", SessionID: "sess-1"},
		{Display: "second prompt", Timestamp: 2000, Project: "/home/user/proj", SessionID: "sess-1"},
		{Display: "only prompt", Timestamp: 3000, Project: "/home/user/other", SessionID: "sess-2"},
	}
	path := writeHistoryFixture(t, entries)

	got, err := ParseHistoryFile(path)
	if err != nil {
		t.Fatalf("ParseHistoryFile() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("map len = %d, want 2", len(got))
	}

	s1, ok := got["sess-1"]
	if !ok {
		t.Fatal("map missing key sess-1")
	}
	if s1.FirstPrompt != "first prompt" {
		t.Errorf("sess-1 FirstPrompt = %q, want %q", s1.FirstPrompt, "first prompt")
	}
	if s1.PromptCount != 2 {
		t.Errorf("sess-1 PromptCount = %d, want 2", s1.PromptCount)
	}

	s2, ok := got["sess-2"]
	if !ok {
		t.Fatal("map missing key sess-2")
	}
	if s2.PromptCount != 1 {
		t.Errorf("sess-2 PromptCount = %d, want 1", s2.PromptCount)
	}
}

func TestParseHistoryFile_MissingFile(t *testing.T) {
	got, err := ParseHistoryFile("/nonexistent/path/history.jsonl")
	if err == nil {
		t.Fatal("ParseHistoryFile() expected error for missing file, got nil")
	}
	if got != nil {
		t.Errorf("ParseHistoryFile() = %v, want nil on error", got)
	}
}

func TestParseHistoryFile_EmptySessionID(t *testing.T) {
	entries := []HistoryEntry{
		{Display: "no session", Timestamp: 1000, Project: "/home/user/proj", SessionID: ""},
		{Display: "valid session", Timestamp: 2000, Project: "/home/user/proj", SessionID: "sess-1"},
	}
	path := writeHistoryFixture(t, entries)

	got, err := ParseHistoryFile(path)
	if err != nil {
		t.Fatalf("ParseHistoryFile() error: %v", err)
	}
	if _, exists := got[""]; exists {
		t.Error("map should not contain empty-string key")
	}
	if _, exists := got["sess-1"]; !exists {
		t.Error("map missing valid key sess-1")
	}
}

func TestParseHistoryFile_MalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/history.jsonl"

	valid := HistoryEntry{Display: "valid", Timestamp: 1000, Project: "/proj", SessionID: "sess-ok"}
	validJSON, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	content := string(validJSON) + "\n" +
		"not json at all\n" +
		`{"incomplete":` + "\n" +
		string(validJSON) + "\n"

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ParseHistoryFile(path)
	if err != nil {
		t.Fatalf("ParseHistoryFile() error: %v", err)
	}
	if _, ok := got["sess-ok"]; !ok {
		t.Error("map missing sess-ok after malformed lines")
	}
	// sess-ok appears twice (both valid lines), so PromptCount should be 2.
	if got["sess-ok"].PromptCount != 2 {
		t.Errorf("sess-ok PromptCount = %d, want 2", got["sess-ok"].PromptCount)
	}
}

func TestParseAllHistory_SortOrder(t *testing.T) {
	entries := []HistoryEntry{
		{Display: "third", Timestamp: 3000, Project: "/proj", SessionID: "s1"},
		{Display: "first", Timestamp: 1000, Project: "/proj", SessionID: "s2"},
		{Display: "second", Timestamp: 2000, Project: "/proj", SessionID: "s3"},
	}
	path := writeHistoryFixture(t, entries)

	got, err := ParseAllHistory(path)
	if err != nil {
		t.Fatalf("ParseAllHistory() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}

	wantOrder := []int64{3000, 2000, 1000}
	for i, want := range wantOrder {
		if got[i].Timestamp != want {
			t.Errorf("got[%d].Timestamp = %d, want %d", i, got[i].Timestamp, want)
		}
	}
}

func TestParseAllHistory_EmptyLines(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/history.jsonl"

	e1 := HistoryEntry{Display: "a", Timestamp: 1000, Project: "/proj", SessionID: "s1"}
	e2 := HistoryEntry{Display: "b", Timestamp: 2000, Project: "/proj", SessionID: "s2"}
	b1, _ := json.Marshal(e1)
	b2, _ := json.Marshal(e2)

	// Insert blank lines between and around entries.
	content := "\n" + string(b1) + "\n\n" + string(b2) + "\n\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ParseAllHistory(path)
	if err != nil {
		t.Fatalf("ParseAllHistory() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (blank lines skipped)", len(got))
	}
}

func TestSearchHistory_Basic(t *testing.T) {
	entries := []HistoryEntry{
		{Display: "deploy the app", Timestamp: 5000, Project: "/proj", SessionID: "s1"},
		{Display: "write tests", Timestamp: 4000, Project: "/proj", SessionID: "s2"},
		{Display: "deploy database", Timestamp: 3000, Project: "/proj", SessionID: "s3"},
		{Display: "fix the bug", Timestamp: 2000, Project: "/proj", SessionID: "s4"},
		{Display: "deploy frontend", Timestamp: 1000, Project: "/proj", SessionID: "s5"},
	}

	results, total := SearchHistory(entries, "deploy", 0, 10)
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}
}

func TestSearchHistory_OffsetLimit(t *testing.T) {
	var entries []HistoryEntry
	for i := 0; i < 10; i++ {
		entries = append(entries, HistoryEntry{
			Display:   fmt.Sprintf("match entry %d", i),
			Timestamp: int64(i + 1),
			Project:   "/proj",
			SessionID: fmt.Sprintf("s%d", i),
		})
	}

	tests := []struct {
		name       string
		offset     int
		limit      int
		wantLen    int
		wantTotal  int
	}{
		{name: "first page", offset: 0, limit: 3, wantLen: 3, wantTotal: 10},
		{name: "last partial", offset: 7, limit: 5, wantLen: 3, wantTotal: 10},
		{name: "past end", offset: 10, limit: 5, wantLen: 0, wantTotal: 10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			results, total := SearchHistory(entries, "match", tc.offset, tc.limit)
			if total != tc.wantTotal {
				t.Errorf("total = %d, want %d", total, tc.wantTotal)
			}
			if tc.wantLen == 0 {
				if results != nil {
					t.Errorf("results = %v, want nil", results)
				}
			} else {
				if len(results) != tc.wantLen {
					t.Errorf("len(results) = %d, want %d", len(results), tc.wantLen)
				}
			}
		})
	}
}

func TestSearchHistory_CaseInsensitive(t *testing.T) {
	entries := []HistoryEntry{
		{Display: "Hello World", Timestamp: 1000, Project: "/proj", SessionID: "s1"},
	}

	results, total := SearchHistory(entries, "hello world", 0, 10)
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
}

func TestSearchHistory_Deduplication(t *testing.T) {
	entries := []HistoryEntry{
		{Display: "deploy app", Timestamp: 2000, Project: "/proj", SessionID: "s1"},
		{Display: "deploy app", Timestamp: 1000, Project: "/proj", SessionID: "s1"},
	}

	results, total := SearchHistory(entries, "deploy", 0, 10)
	if total != 1 {
		t.Errorf("total = %d, want 1 (deduplication by sessionID+display)", total)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
}

func TestSearchHistory_NoMatch(t *testing.T) {
	entries := []HistoryEntry{
		{Display: "write tests", Timestamp: 1000, Project: "/proj", SessionID: "s1"},
		{Display: "fix bug", Timestamp: 2000, Project: "/proj", SessionID: "s2"},
	}

	results, total := SearchHistory(entries, "deploy", 0, 10)
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

func TestEncodeProjectPath(t *testing.T) {
	entries := []HistoryEntry{
		{Display: "some task", Timestamp: 1000, Project: "/home/user/project", SessionID: "s1"},
	}

	results, total := SearchHistory(entries, "some task", 0, 10)
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	wantProjectID := "-home-user-project"
	if results[0].ProjectID != wantProjectID {
		t.Errorf("ProjectID = %q, want %q", results[0].ProjectID, wantProjectID)
	}
}
