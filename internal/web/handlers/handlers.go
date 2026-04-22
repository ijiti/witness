package handlers

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/ijiti/witness/internal/discovery"
)

// Handlers holds shared dependencies for all HTTP handlers.
type Handlers struct {
	disc  *discovery.Discoverer
	pages map[string]*template.Template
}

// New creates a new Handlers instance.
func New(disc *discovery.Discoverer, pages map[string]*template.Template) *Handlers {
	return &Handlers{disc: disc, pages: pages}
}

// render executes the named page template. For HTMX requests, only the
// "content" block is rendered.
func (h *Handlers) render(w http.ResponseWriter, r *http.Request, page string, data map[string]any) {
	t, ok := h.pages[page]
	if !ok {
		log.Printf("template %q not found", page)
		http.Error(w, "page not found", http.StatusInternalServerError)
		return
	}

	tmplName := "base"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}

	// Inject project list for sidebar on full page loads.
	if tmplName == "base" {
		if _, exists := data["Projects"]; !exists {
			if projects, err := h.disc.ListProjects(); err == nil {
				data["Projects"] = projects
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template execute error: %v", err)
	}
}

// CheckETag returns true (and writes 304) if the client's If-None-Match
// header matches the given mtime. Call before rendering.
func CheckETag(w http.ResponseWriter, r *http.Request, mtime time.Time) bool {
	if mtime.IsZero() {
		return false
	}
	etag := fmt.Sprintf(`"%x"`, mtime.UnixNano())
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache") // revalidate every request
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

// Health returns a 200 OK.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
