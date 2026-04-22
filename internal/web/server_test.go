package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ijiti/witness/internal/discovery"
	"github.com/ijiti/witness/internal/web/handlers"
)

// setupTestEnv creates a minimal ~/.claude/ directory tree for handler tests.
func setupTestEnv(t *testing.T) *handlers.Handlers {
	t.Helper()
	dir := t.TempDir()

	// Create projects directory with a sample project and session.
	projDir := filepath.Join(dir, "projects", "-test-project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a minimal session JSONL file.
	sessFile := filepath.Join(projDir, "sess-1.jsonl")
	now := time.Now().UTC()
	lines := []string{
		marshalJSON(map[string]interface{}{
			"type":      "user",
			"uuid":      "u1",
			"sessionId": "sess-1",
			"timestamp": now.Format(time.RFC3339Nano),
			"message":   map[string]interface{}{"role": "user", "content": "hello"},
		}),
		marshalJSON(map[string]interface{}{
			"type":      "assistant",
			"uuid":      "a1",
			"sessionId": "sess-1",
			"timestamp": now.Add(time.Second).Format(time.RFC3339Nano),
			"message": map[string]interface{}{
				"role":    "assistant",
				"id":      "msg-1",
				"model":   "claude-opus-4-6",
				"content": []map[string]interface{}{{"type": "text", "text": "world"}},
				"usage":   map[string]interface{}{"input_tokens": 100, "output_tokens": 50},
			},
		}),
	}
	if err := os.WriteFile(sessFile, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write history.jsonl for search.
	histFile := filepath.Join(dir, "history.jsonl")
	histLines := []string{
		marshalJSON(map[string]interface{}{
			"display":   "hello world test",
			"timestamp": now.UnixMilli(),
			"project":   "/test/project",
			"sessionId": "sess-1",
		}),
		marshalJSON(map[string]interface{}{
			"display":   "another session prompt",
			"timestamp": now.Add(-time.Hour).UnixMilli(),
			"project":   "/test/project",
			"sessionId": "sess-2",
		}),
	}
	if err := os.WriteFile(histFile, []byte(strings.Join(histLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write stats-cache.json.
	statsFile := filepath.Join(dir, "stats-cache.json")
	stats := map[string]interface{}{
		"dailyStats": map[string]interface{}{
			"2026-02-20": map[string]interface{}{
				"messages":  10,
				"sessions":  2,
				"toolCalls": 5,
				"tokensPerModel": map[string]interface{}{
					"claude-opus-4-6": map[string]interface{}{
						"input":       1000,
						"output":      500,
						"cacheRead":   200,
						"cacheCreate": 100,
					},
				},
			},
		},
		"hourlyActivity": map[string]int{
			"10": 5,
			"14": 3,
		},
	}
	statsJSON, _ := json.MarshalIndent(stats, "", "  ")
	if err := os.WriteFile(statsFile, statsJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	disc := discovery.NewDiscoverer(filepath.Join(dir, "projects"))
	pages := ParseTemplates()
	return handlers.New(disc, pages)
}

func marshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// TestCheckETag_Match tests that matching ETag returns 304.
func TestCheckETag_Match(t *testing.T) {
	mtime := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	etag := fmt.Sprintf(`"%x"`, mtime.UnixNano())

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("If-None-Match", etag)
	w := httptest.NewRecorder()

	if !handlers.CheckETag(w, r, mtime) {
		t.Error("CheckETag returned false, want true for matching ETag")
	}
	if w.Code != http.StatusNotModified {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotModified)
	}
}

// TestCheckETag_NoMatch tests that non-matching ETag returns false.
func TestCheckETag_NoMatch(t *testing.T) {
	mtime := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("If-None-Match", `"wrong"`)
	w := httptest.NewRecorder()

	if handlers.CheckETag(w, r, mtime) {
		t.Error("CheckETag returned true, want false for non-matching ETag")
	}
	if w.Header().Get("ETag") == "" {
		t.Error("ETag header not set")
	}
}

// TestCheckETag_NoHeader tests missing If-None-Match header.
func TestCheckETag_NoHeader(t *testing.T) {
	mtime := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)

	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	if handlers.CheckETag(w, r, mtime) {
		t.Error("CheckETag returned false, want false for missing header")
	}
	if w.Header().Get("ETag") == "" {
		t.Error("ETag header not set")
	}
}

// TestCheckETag_ZeroTime tests zero mtime skips ETag.
func TestCheckETag_ZeroTime(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	if handlers.CheckETag(w, r, time.Time{}) {
		t.Error("CheckETag returned true for zero mtime")
	}
}

// TestHealth tests the health endpoint.
func TestHealth(t *testing.T) {
	h := setupTestEnv(t)
	r := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.Health(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

// TestDashboard tests the dashboard renders successfully.
func TestDashboard(t *testing.T) {
	h := setupTestEnv(t)
	r := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()

	h.Dashboard(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("dashboard returned empty body")
	}
}

// TestSearch_WithResults tests search returns matching results.
func TestSearch_WithResults(t *testing.T) {
	h := setupTestEnv(t)
	r := httptest.NewRequest("GET", "/search?q=hello", nil)
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()

	h.Search(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "hello world test") {
		t.Error("search results should contain 'hello world test'")
	}
}

// TestSearch_NoResults tests search with no matches.
func TestSearch_NoResults(t *testing.T) {
	h := setupTestEnv(t)
	r := httptest.NewRequest("GET", "/search?q=nonexistent", nil)
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()

	h.Search(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "No results found") {
		t.Error("should show 'No results found'")
	}
}
