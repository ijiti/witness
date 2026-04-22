package handlers

import (
	"net/http"
)

// Dashboard renders the usage analytics dashboard from stats-cache.json.
func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	// ETag based on stats-cache.json mtime.
	if CheckETag(w, r, h.disc.StatsFileMtime()) {
		return
	}

	stats := h.disc.GetStats()

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
	}

	h.render(w, r, "dashboard", data)
}
