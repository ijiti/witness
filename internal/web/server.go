package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ijiti/witness/internal/costlog"
	"github.com/ijiti/witness/internal/format"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ijiti/witness/internal/claudelog"
	"github.com/ijiti/witness/internal/discovery"
	"github.com/ijiti/witness/internal/render"
	"github.com/ijiti/witness/internal/web/handlers"
)

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// NewServer creates an HTTP server with all routes configured.
func NewServer(disc *discovery.Discoverer, addr string) *http.Server {
	pages := ParseTemplates()
	h := handlers.New(disc, pages)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(securityHeaders)

	// Serve vendored static assets (htmx, tailwind CSS).
	staticContent, _ := fs.Sub(staticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticContent))))

	r.Get("/", h.Dashboard)
	r.Get("/dashboard", h.Dashboard)
	r.Get("/health", h.Health)
	r.Get("/search", h.Search)
	r.Get("/compare", h.Compare)
	r.Get("/compare/sessions", h.CompareSessions)
	r.Get("/projects/{projectID}", h.SessionList)
	r.Get("/projects/{projectID}/sessions/{sessionID}", h.SessionDetail)
	r.Get("/projects/{projectID}/sessions/{sessionID}/subagents/{agentID}", h.SubagentDetail)
	r.Get("/projects/{projectID}/sessions/{sessionID}/plan", h.SessionPlan)

	// Lazy-load turns for large sessions (session 5).
	r.Get("/projects/{projectID}/sessions/{sessionID}/turns", h.SessionTurns)

	// SSE streaming endpoints (session 3: live monitoring).
	r.Get("/projects/{projectID}/stream", h.StreamProject)
	r.Get("/projects/{projectID}/sessions/{sessionID}/stream", h.StreamSession)

	return &http.Server{
		Addr:        addr,
		Handler:     r,
		ReadTimeout: 30 * time.Second,
		// WriteTimeout must be 0 for SSE — Go enforces it from
		// first byte written, killing long-lived event streams.
		IdleTimeout: 120 * time.Second,
	}
}

// ParseTemplates builds the full template map from the embedded filesystem.
// Exported for use by handler tests.
func ParseTemplates() map[string]*template.Template {
	funcMap := template.FuncMap{
		"formatTime":     formatTime,
		"formatDuration": formatDuration,
		"formatTokens":   formatTokens,
		"formatBytes":    format.Bytes,
		"formatCost":     costlog.FormatCost,
		"truncate":       format.Truncate,
		"toolColor":      toolColor,
		"agentBg":        agentBg,
		"add":            func(a, b int) int { return a + b },
		"sub":            func(a, b int) int { return a - b },

		// Tool-specific template helpers.
		"isBash": func(tc claudelog.ToolCall) bool {
			_, ok := tc.Input.(claudelog.ToolInputBash)
			return ok
		},
		"bashCommand": func(tc claudelog.ToolCall) string {
			if v, ok := tc.Input.(claudelog.ToolInputBash); ok {
				return v.Command
			}
			return ""
		},
		"bashDescription": func(tc claudelog.ToolCall) string {
			if v, ok := tc.Input.(claudelog.ToolInputBash); ok {
				return v.Description
			}
			return ""
		},
		"isRead": func(tc claudelog.ToolCall) bool {
			_, ok := tc.Input.(claudelog.ToolInputRead)
			return ok
		},
		"readFilePath": func(tc claudelog.ToolCall) string {
			if v, ok := tc.Input.(claudelog.ToolInputRead); ok {
				return v.FilePath
			}
			return ""
		},
		"isEdit": func(tc claudelog.ToolCall) bool {
			_, ok := tc.Input.(claudelog.ToolInputEdit)
			return ok
		},
		"editOldString": func(tc claudelog.ToolCall) string {
			if v, ok := tc.Input.(claudelog.ToolInputEdit); ok {
				return v.OldString
			}
			return ""
		},
		"editNewString": func(tc claudelog.ToolCall) string {
			if v, ok := tc.Input.(claudelog.ToolInputEdit); ok {
				return v.NewString
			}
			return ""
		},
		"editFilePath": func(tc claudelog.ToolCall) string {
			if v, ok := tc.Input.(claudelog.ToolInputEdit); ok {
				return v.FilePath
			}
			return ""
		},
		"isWrite": func(tc claudelog.ToolCall) bool {
			_, ok := tc.Input.(claudelog.ToolInputWrite)
			return ok
		},
		"writeFilePath": func(tc claudelog.ToolCall) string {
			if v, ok := tc.Input.(claudelog.ToolInputWrite); ok {
				return v.FilePath
			}
			return ""
		},
		"writeContent": func(tc claudelog.ToolCall) string {
			if v, ok := tc.Input.(claudelog.ToolInputWrite); ok {
				return v.Content
			}
			return ""
		},

		// Task/subagent helpers.
		"isTask": func(tc claudelog.ToolCall) bool {
			_, ok := tc.Input.(claudelog.ToolInputTask)
			return ok
		},
		"taskInput": func(tc claudelog.ToolCall) claudelog.ToolInputTask {
			if v, ok := tc.Input.(claudelog.ToolInputTask); ok {
				return v
			}
			return claudelog.ToolInputTask{}
		},
		"formatDurationMs": func(ms int64) string {
			d := time.Duration(ms) * time.Millisecond
			return formatDuration(d)
		},

		// Rendering functions.
		"renderDiff":    render.RenderDiffHTML,
		"highlightCode": render.HighlightCode,

		// Dashboard chart helpers.
		"mul": func(a, b int) int { return a * b },
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i
			}
			return s
		},
		"shortDate": func(date string) string {
			if len(date) >= 10 {
				return date[5:10] // "2026-02-14" → "02-14"
			}
			return date
		},
		"hourCount": func(counts map[string]int, h int) int {
			return counts[strconv.Itoa(h)]
		},
		"hourColor": func(count, max int) string {
			if max == 0 || count == 0 {
				return "fill-gray-800"
			}
			pct := float64(count) / float64(max)
			switch {
			case pct > 0.75:
				return "fill-blue-400"
			case pct > 0.5:
				return "fill-blue-500"
			case pct > 0.25:
				return "fill-blue-600"
			default:
				return "fill-blue-700/70"
			}
		},
		"pctWidth": func(val, max int) int {
			if max == 0 {
				return 0
			}
			w := val * 100 / max
			if w < 1 && val > 0 {
				w = 1
			}
			return w
		},
	}

	base := template.Must(
		template.New("").Funcs(funcMap).ParseFS(templateFS,
			"templates/layouts/base.html",
			"templates/partials/*.html",
		),
	)

	pages := map[string]*template.Template{}

	pageFiles, err := fs.Glob(templateFS, "templates/pages/*.html")
	if err != nil {
		panic(fmt.Sprintf("failed to glob page templates: %v", err))
	}

	for _, pf := range pageFiles {
		name := strings.TrimSuffix(filepath.Base(pf), ".html")
		t := template.Must(template.Must(base.Clone()).ParseFS(templateFS, pf))
		pages[name] = t
	}

	return pages
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return ""
	}
	sec := int(d.Seconds())
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	min := sec / 60
	rem := sec % 60
	if min < 60 {
		return fmt.Sprintf("%dm %ds", min, rem)
	}
	hr := min / 60
	min = min % 60
	return fmt.Sprintf("%dh %dm", hr, min)
}

func formatTokens(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}


func toolColor(name string) string {
	switch name {
	case "Read", "Glob", "Grep":
		return "blue"
	case "Bash":
		return "yellow"
	case "Edit", "Write":
		return "green"
	case "Task", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet", "TaskOutput":
		return "cyan"
	case "WebFetch", "WebSearch":
		return "purple"
	case "AskUserQuestion", "EnterPlanMode", "ExitPlanMode":
		return "orange"
	default:
		return "gray"
	}
}

func agentBg(color string) string {
	switch color {
	case "orange":
		return "bg-orange-900/50 text-orange-300"
	case "cyan":
		return "bg-cyan-900/50 text-cyan-300"
	case "blue":
		return "bg-blue-900/50 text-blue-300"
	case "green":
		return "bg-green-900/50 text-green-300"
	case "purple":
		return "bg-purple-900/50 text-purple-300"
	case "red":
		return "bg-red-900/50 text-red-300"
	case "yellow":
		return "bg-yellow-900/50 text-yellow-300"
	default:
		return "bg-gray-700 text-gray-300"
	}
}
