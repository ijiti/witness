package handlers

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ijiti/witness/internal/build"
	"github.com/ijiti/witness/internal/discovery"
)

// Handlers holds shared dependencies for all HTTP handlers.
type Handlers struct {
	disc    *discovery.Discoverer
	pages   map[string]*template.Template
	buildNS string // mixed into all ETags; changes on every new binary
}

// New creates a new Handlers instance. It reads the binary's own mtime once
// so that every `go build` (even without -ldflags) produces a distinct ETag
// namespace, preventing stale-layout cache hits after binary updates.
func New(disc *discovery.Discoverer, pages map[string]*template.Template) *Handlers {
	ns := build.BuildID
	if ns == "dev" {
		// Development build: use the binary's mtime as a unique suffix so
		// that repeated `go build` still busts stale browser caches.
		if exe, err := os.Executable(); err == nil {
			if fi, err := os.Stat(exe); err == nil {
				ns = fmt.Sprintf("dev-%x", fi.ModTime().UnixNano())
			}
		}
	}
	return &Handlers{disc: disc, pages: pages, buildNS: ns}
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
		// Always derive ProjectGroups from Projects so the sidebar gets a
		// consistent grouped view regardless of which handler set Projects.
		if _, exists := data["Projects"]; !exists {
			if projects, err := h.disc.ListProjects(); err == nil {
				data["Projects"] = projects
			}
		}
		if _, exists := data["ProjectGroups"]; !exists {
			if projects, ok := data["Projects"].([]discovery.Project); ok {
				data["ProjectGroups"] = discovery.GroupProjects(projects)
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template execute error: %v", err)
	}
}

// CheckETag returns true (and writes 304) if the client's If-None-Match
// header matches a tag derived from the given mtime and the build identity.
// Incorporating BuildID means every new binary release busts previously cached
// layouts even when the underlying file mtime has not changed.
func (h *Handlers) CheckETag(w http.ResponseWriter, r *http.Request, mtime time.Time) bool {
	if mtime.IsZero() {
		return false
	}
	etag := fmt.Sprintf(`"%s-%x"`, h.buildNS, mtime.UnixNano())
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache") // revalidate every request
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

// renderError writes an error page using the witness chrome (sidebar + layout).
// It sets the HTTP status code before rendering so curl/HTMX consumers see the
// correct status even though the body is HTML.
func (h *Handlers) renderError(w http.ResponseWriter, r *http.Request, status int, headline, detail string) {
	w.WriteHeader(status)
	h.render(w, r, "error", map[string]any{
		"PageTitle":  fmt.Sprintf("witness - %d", status),
		"StatusCode": status,
		"Headline":   headline,
		"Detail":     detail,
	})
}

// Health returns a 200 OK.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
