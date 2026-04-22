package handlers

import (
	"log"
	"net/http"
	"strconv"
)

// Search handles cross-session search queries with pagination.
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	projects, _ := h.disc.ListProjects()
	data := map[string]interface{}{
		"Query":     query,
		"PageTitle": "witness - search",
		"Projects":  projects,
	}

	if query != "" {
		results, total, err := h.disc.SearchSessions(query, offset, limit)
		if err != nil {
			log.Printf("search error: %v", err)
		}
		data["Results"] = results
		data["Total"] = total
		data["Offset"] = offset
		data["Limit"] = limit
		data["HasMore"] = offset+limit < total
		data["NextOffset"] = offset + limit
		data["ShowEnd"] = offset + len(results)
	}

	h.render(w, r, "search", data)
}
