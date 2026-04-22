package claudelog

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"
)

// HistoryEntry represents one line from ~/.claude/history.jsonl.
type HistoryEntry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"` // Unix milliseconds
	Project   string `json:"project"`
	SessionID string `json:"sessionId"`
}

// SessionHistory holds the first prompt and timestamp for a session.
type SessionHistory struct {
	FirstPrompt string
	StartTime   time.Time
	Project     string
	PromptCount int
}

// ParseHistoryFile reads history.jsonl and returns a map of sessionId → SessionHistory.
// For each sessionId, it keeps the first entry's display text as the prompt.
func ParseHistoryFile(path string) (map[string]*SessionHistory, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]*SessionHistory)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	for scanner.Scan() {
		var entry HistoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.SessionID == "" {
			continue
		}
		if _, exists := result[entry.SessionID]; !exists {
			result[entry.SessionID] = &SessionHistory{
				FirstPrompt: entry.Display,
				StartTime:   time.UnixMilli(entry.Timestamp),
				Project:     entry.Project,
			}
		}
		result[entry.SessionID].PromptCount++
	}
	return result, scanner.Err()
}

// SearchResult is a matched history entry.
type SearchResult struct {
	SessionID string
	ProjectID string // encoded project directory name
	Project   string // raw project path
	Display   string
	Timestamp time.Time
}

// ParseAllHistory reads all history.jsonl entries sorted by timestamp desc.
func ParseAllHistory(path string) ([]HistoryEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var entries []HistoryEntry
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e HistoryEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Sort by timestamp descending.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp > entries[j].Timestamp
	})
	return entries, nil
}

// SearchHistory performs case-insensitive substring search across entries.
// Returns matching results sliced by offset/limit and the total match count.
func SearchHistory(entries []HistoryEntry, query string, offset, limit int) ([]SearchResult, int) {
	query = strings.ToLower(query)
	seen := make(map[string]bool)
	var all []SearchResult

	for _, e := range entries {
		if !strings.Contains(strings.ToLower(e.Display), query) {
			continue
		}
		key := e.SessionID + "|" + e.Display
		if seen[key] {
			continue
		}
		seen[key] = true
		all = append(all, SearchResult{
			SessionID: e.SessionID,
			ProjectID: encodeProjectPath(e.Project),
			Project:   e.Project,
			Display:   e.Display,
			Timestamp: time.UnixMilli(e.Timestamp),
		})
	}

	total := len(all)
	if offset >= total {
		return nil, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total
}

// encodeProjectPath converts a filesystem path to a Claude project directory name.
func encodeProjectPath(path string) string {
	return strings.ReplaceAll(path, "/", "-")
}
