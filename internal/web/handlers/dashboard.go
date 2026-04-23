package handlers

import (
	"net/http"
)

// Dashboard renders the usage analytics dashboard.
//
// It prefers fresh in-memory stats (recomputed from JSONL on startup and after
// file-watcher events). While the background compute is still running on first
// hit, it falls back to the file-based stats-cache.json so the page never
// returns empty.
func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	// Prefer fresh stats (computed from actual JSONL files).
	stats := h.disc.GetFreshStats()
	freshTime := h.disc.FreshStatsTime()

	if stats != nil {
		// ETag on the fresh compute timestamp — changes whenever we recompute.
		if h.CheckETag(w, r, freshTime) {
			return
		}
	} else {
		// Fresh compute not done yet — fall back to stats-cache.json.
		stats = h.disc.GetStats()
		if h.CheckETag(w, r, h.disc.StatsFileMtime()) {
			return
		}
	}

	projects, _ := h.disc.ListProjects()

	data := map[string]any{
		"PageTitle": "witness - dashboard",
		"Projects":  projects,
	}

	if stats != nil {
		data["Stats"] = stats
		data["ModelSummaries"] = stats.ModelSummaries()
		data["MaxMessages"] = stats.MaxDailyMessages()
		data["MaxToolCalls"] = stats.MaxDailyToolCalls()
		data["MaxHourCount"] = stats.MaxHourCount()
		data["MaxDailyCost"] = stats.MaxDailyCost()
	}

	h.render(w, r, "dashboard", data)
}
