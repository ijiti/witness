package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// StreamSession is an SSE endpoint that streams live session updates.
// Clients receive new turns as rendered HTML partials.
func (h *Handlers) StreamSession(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	sessionID := chi.URLParam(r, "sessionID")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering if present

	// Send initial keepalive.
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	clientID, events := h.disc.Broadcaster.Subscribe()
	defer h.disc.Broadcaster.Unsubscribe(clientID)

	// Track the last known turn count so we can diff.
	lastTurnCount := 0
	if sess, err := h.disc.GetSession(projectID, sessionID); err == nil {
		lastTurnCount = len(sess.Turns)
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}

			// Only process events for our session.
			if ev.ProjectID != projectID || ev.SessionID != sessionID {
				// Also send session-list updates for any change in our project.
				if ev.ProjectID == projectID {
					fmt.Fprintf(w, "event: session-list\ndata: updated\n\n")
					flusher.Flush()
				}
				continue
			}

			// Re-parse the session to find new turns.
			sess, err := h.disc.GetSession(projectID, sessionID)
			if err != nil {
				log.Printf("stream: re-parse %s/%s: %v", projectID, sessionID, err)
				continue
			}

			newTurnCount := len(sess.Turns)
			if newTurnCount < lastTurnCount {
				// Turn count decreased (shouldn't happen) — reset.
				lastTurnCount = newTurnCount
				continue
			}

			if newTurnCount == lastTurnCount {
				// Same count — the last turn may have been updated
				// (assistant streaming adds content blocks to the same turn).
				if newTurnCount > 0 {
					lastTurn := sess.Turns[newTurnCount-1]
					html := h.renderPartial("turn", lastTurn)
					if html != "" {
						fmt.Fprintf(w, "event: turn-update\ndata: %s\n\n", html)
						flusher.Flush()
					}
				}
				html := h.renderPartial("session_header", sess)
				if html != "" {
					fmt.Fprintf(w, "event: header\ndata: %s\n\n", html)
					flusher.Flush()
				}
				continue
			}

			// New turns appeared. Send each one.
			for i := lastTurnCount; i < newTurnCount; i++ {
				turn := sess.Turns[i]
				html := h.renderPartial("turn", turn)
				if html != "" {
					fmt.Fprintf(w, "event: turn\ndata: %s\n\n", html)
					flusher.Flush()
				}
			}

			// Also send updated header (token counts, cost, etc.).
			html := h.renderPartial("session_header", sess)
			if html != "" {
				fmt.Fprintf(w, "event: header\ndata: %s\n\n", html)
				flusher.Flush()
			}

			lastTurnCount = newTurnCount
		}
	}
}

// StreamProject is an SSE endpoint for project-level updates (session list changes).
func (h *Handlers) StreamProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	clientID, events := h.disc.Broadcaster.Subscribe()
	defer h.disc.Broadcaster.Unsubscribe(clientID)

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.ProjectID != projectID {
				continue
			}
			// Any change in the project triggers a session list refresh.
			fmt.Fprintf(w, "event: refresh\ndata: %s\n\n", ev.SessionID)
			flusher.Flush()
		}
	}
}

// StreamActivity is a global SSE endpoint that emits every watcher event as a
// compact JSON payload so the sidebar can light "currently writing" dots on
// any project/session without reloading. Runs independently of per-view SSE
// streams: the client keeps one activity connection open for the life of the
// page while per-view streams come and go during HTMX navigation.
func (h *Handlers) StreamActivity(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	clientID, events := h.disc.Broadcaster.Subscribe()
	defer h.disc.Broadcaster.Unsubscribe(clientID)

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.ProjectID == "" {
				continue
			}
			payload, err := json.Marshal(struct {
				ProjectID string `json:"projectID"`
				SessionID string `json:"sessionID,omitempty"`
			}{ev.ProjectID, ev.SessionID})
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: activity\ndata: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

// renderPartial executes a named template partial and returns the HTML as a
// single-line string suitable for SSE data fields.
func (h *Handlers) renderPartial(name string, data interface{}) string {
	var buf bytes.Buffer
	// Look for the partial in any page template (they all share the same partials).
	var t *template.Template
	for _, pt := range h.pages {
		t = pt.Lookup(name)
		if t != nil {
			break
		}
	}
	if t == nil {
		log.Printf("stream: partial %q not found", name)
		return ""
	}
	if err := t.Execute(&buf, data); err != nil {
		log.Printf("stream: render %q: %v", name, err)
		return ""
	}
	// SSE data lines cannot contain newlines. Encode as single line.
	return sseEncode(buf.String())
}

// sseEncode converts multi-line HTML to SSE-compatible format.
// Each line after the first gets its own "data: " prefix.
func sseEncode(s string) string {
	var buf bytes.Buffer
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			buf.WriteByte('\n')
			buf.WriteString("data: ")
		} else {
			buf.WriteByte(s[i])
		}
	}
	return buf.String()
}
