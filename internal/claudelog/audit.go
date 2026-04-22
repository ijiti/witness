package claudelog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// AuditEvent is a unified audit event for display.
type AuditEvent struct {
	Timestamp time.Time
	Type      string // "tool", "canary", "sanitizer"
	Tool      string
	Summary   string
	Severity  string // "" for tool, severity for sanitizer, "critical" for canary
}

// rawAuditEntry is the JSON shape of standard audit log entries.
type rawAuditEntry struct {
	Timestamp string `json:"timestamp"`
	Tool      string `json:"tool"`
	Args      string `json:"args"`
	Session   string `json:"session"`
	Result    string `json:"result"`
}

// rawCanaryEntry is the JSON shape of canary audit entries.
type rawCanaryEntry struct {
	Timestamp     string `json:"timestamp"`
	SessionID     string `json:"session_id"`
	Command       string `json:"command"`
	MatchedCanary string `json:"matched_canary"`
	Label         string `json:"label"`
}

// rawSanitizerEntry is the JSON shape of content-sanitizer audit entries.
type rawSanitizerEntry struct {
	Timestamp  string `json:"timestamp"`
	Severity   string `json:"severity"`
	Tool       string `json:"tool"`
	Source     string `json:"source"`
	Detections string `json:"detections"`
	Session    string `json:"session"`
}

// LoadAuditForSession reads audit files covering the session's date range,
// filters by session ID, and returns unified AuditEvents sorted by time.
func LoadAuditForSession(auditDir, sessionID string, startTime, endTime time.Time) []AuditEvent {
	if auditDir == "" || sessionID == "" || startTime.IsZero() {
		return nil
	}

	// Determine date range.
	startDate := startTime.Local().Format("2006-01-02")
	endDate := endTime.Local().Format("2006-01-02")

	var events []AuditEvent

	// Parse dates and iterate.
	d, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil
	}
	end = end.AddDate(0, 0, 1) // inclusive

	for !d.After(end) {
		dateStr := d.Format("2006-01-02")

		// Standard audit file.
		events = append(events, parseStandardAudit(
			filepath.Join(auditDir, dateStr+".jsonl"), sessionID)...)

		// Canary audit file.
		events = append(events, parseCanaryAudit(
			filepath.Join(auditDir, "canary-"+dateStr+".jsonl"), sessionID)...)

		// Content sanitizer audit file.
		events = append(events, parseSanitizerAudit(
			filepath.Join(auditDir, "content-sanitizer-"+dateStr+".jsonl"), sessionID)...)

		d = d.AddDate(0, 0, 1)
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	return events
}

// scanJSONL opens a JSONL file and calls fn for each non-empty line.
func scanJSONL(path string, fn func([]byte)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if line := scanner.Bytes(); len(line) > 0 {
			fn(line)
		}
	}
}

func parseStandardAudit(path, sessionID string) []AuditEvent {
	var events []AuditEvent
	scanJSONL(path, func(line []byte) {
		var e rawAuditEntry
		if err := json.Unmarshal(line, &e); err != nil || e.Session != sessionID {
			return
		}
		t, _ := time.Parse(time.RFC3339, e.Timestamp)
		summary := e.Tool
		if len(e.Args) > 80 {
			summary += ": " + e.Args[:77] + "..."
		} else if e.Args != "" && e.Args != "{}" {
			summary += ": " + e.Args
		}
		events = append(events, AuditEvent{
			Timestamp: t,
			Type:      "tool",
			Tool:      e.Tool,
			Summary:   summary,
		})
	})
	return events
}

func parseCanaryAudit(path, sessionID string) []AuditEvent {
	var events []AuditEvent
	scanJSONL(path, func(line []byte) {
		var e rawCanaryEntry
		if err := json.Unmarshal(line, &e); err != nil || e.SessionID != sessionID {
			return
		}
		t, _ := time.Parse(time.RFC3339, e.Timestamp)
		events = append(events, AuditEvent{
			Timestamp: t,
			Type:      "canary",
			Tool:      "canary",
			Summary:   fmt.Sprintf("Canary detected: %s (%s)", e.Label, e.MatchedCanary),
			Severity:  "critical",
		})
	})
	return events
}

func parseSanitizerAudit(path, sessionID string) []AuditEvent {
	var events []AuditEvent
	scanJSONL(path, func(line []byte) {
		var e rawSanitizerEntry
		if err := json.Unmarshal(line, &e); err != nil || e.Session != sessionID {
			return
		}
		t, _ := time.Parse(time.RFC3339, e.Timestamp)
		events = append(events, AuditEvent{
			Timestamp: t,
			Type:      "sanitizer",
			Tool:      e.Tool,
			Summary:   fmt.Sprintf("%s: %s in %s", e.Severity, e.Detections, e.Source),
			Severity:  e.Severity,
		})
	})
	return events
}
