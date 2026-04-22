package handlers

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ijiti/witness/internal/claudelog"
)

// ComparisonData holds computed metrics for two sessions side-by-side.
type ComparisonData struct {
	SessionA *claudelog.Session
	SessionB *claudelog.Session

	// Deltas (B - A).
	DeltaTurns        int
	DeltaCost         float64
	DeltaInputTokens  int
	DeltaOutputTokens int
	DeltaDuration     time.Duration
	DurationA         time.Duration
	DurationB         time.Duration

	// Tool usage frequency.
	ToolCountsA  map[string]int
	ToolCountsB  map[string]int
	AllTools     []string // union, sorted by combined frequency desc
	MaxToolCount int      // for bar scaling via pctWidth
}

// Compare renders the session comparison page.
// Without query params: picker mode. With ?a=proj/sess&b=proj/sess: comparison view.
func (h *Handlers) Compare(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"PageTitle": "witness - compare",
	}

	paramA := r.URL.Query().Get("a")
	paramB := r.URL.Query().Get("b")

	if paramA == "" || paramB == "" {
		// Picker mode.
		h.render(w, r, "compare", data)
		return
	}

	sessA, err := h.loadSessionFromParam(paramA)
	if err != nil {
		log.Printf("compare: load session A %q: %v", paramA, err)
		http.Error(w, "session A not found", http.StatusNotFound)
		return
	}
	sessB, err := h.loadSessionFromParam(paramB)
	if err != nil {
		log.Printf("compare: load session B %q: %v", paramB, err)
		http.Error(w, "session B not found", http.StatusNotFound)
		return
	}

	h.disc.EnrichTitle(sessA)
	h.disc.EnrichTitle(sessB)

	comp := buildComparison(sessA, sessB)

	data["Comparison"] = comp
	data["ParamA"] = paramA
	data["ParamB"] = paramB

	h.render(w, r, "compare", data)
}

// CompareSessions returns <option> elements for the session picker dropdown.
// Called via HTMX when the user selects a project.
func (h *Handlers) CompareSessions(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project")
	if projectID == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<option value="">Select session...</option>`))
		return
	}

	sessions, err := h.disc.ListSessions(projectID)
	if err != nil {
		log.Printf("compare sessions: list %s: %v", projectID, err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<option value="">No sessions found</option>`))
		return
	}

	t, ok := h.pages["compare"]
	if !ok {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "session_options", map[string]any{
		"Sessions":  sessions,
		"ProjectID": projectID,
	}); err != nil {
		log.Printf("compare sessions template: %v", err)
	}
}

func (h *Handlers) loadSessionFromParam(param string) (*claudelog.Session, error) {
	projectID, sessionID, ok := strings.Cut(param, "/")
	if !ok {
		return nil, fmt.Errorf("invalid session param: %s", param)
	}
	return h.disc.GetSession(projectID, sessionID)
}

func sessionDuration(s *claudelog.Session) time.Duration {
	if s.EndTime.IsZero() || s.StartTime.IsZero() {
		return 0
	}
	return s.EndTime.Sub(s.StartTime)
}

func buildComparison(a, b *claudelog.Session) *ComparisonData {
	durA := sessionDuration(a)
	durB := sessionDuration(b)

	comp := &ComparisonData{
		SessionA:          a,
		SessionB:          b,
		DeltaTurns:        len(b.Turns) - len(a.Turns),
		DeltaCost:         b.TotalCost - a.TotalCost,
		DeltaInputTokens:  b.TotalInputTokens - a.TotalInputTokens,
		DeltaOutputTokens: b.TotalOutputTokens - a.TotalOutputTokens,
		DeltaDuration:     durB - durA,
		DurationA:         durA,
		DurationB:         durB,
		ToolCountsA:       countTools(a),
		ToolCountsB:       countTools(b),
	}

	// Build sorted union of tool names.
	combined := make(map[string]int)
	for t, c := range comp.ToolCountsA {
		combined[t] += c
	}
	for t, c := range comp.ToolCountsB {
		combined[t] += c
	}
	for _, c := range combined {
		if c > comp.MaxToolCount {
			comp.MaxToolCount = c
		}
	}
	for t := range combined {
		comp.AllTools = append(comp.AllTools, t)
	}
	sort.Slice(comp.AllTools, func(i, j int) bool {
		return combined[comp.AllTools[i]] > combined[comp.AllTools[j]]
	})

	return comp
}

func countTools(s *claudelog.Session) map[string]int {
	counts := make(map[string]int)
	for _, turn := range s.Turns {
		for _, tc := range turn.ToolCalls {
			counts[tc.Name]++
		}
	}
	return counts
}
