package handlers

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// SessionList renders the session list for a project.
func (h *Handlers) SessionList(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	sessions, err := h.disc.ListSessions(projectID)
	if err != nil {
		log.Printf("list sessions for %s: %v", projectID, err)
		if os.IsNotExist(err) {
			h.renderError(w, r, http.StatusNotFound, "Project not found", "Project "+projectID+" does not exist")
		} else {
			h.renderError(w, r, http.StatusBadRequest, "Bad request", "Could not list sessions for project "+projectID)
		}
		return
	}

	projects, _ := h.disc.ListProjects()

	// Find current project for display.
	var projectName string
	for _, p := range projects {
		if p.ID == projectID {
			projectName = p.DisplayName
			break
		}
	}

	h.render(w, r, "index", map[string]any{
		"Projects":      projects,
		"Sessions":      sessions,
		"ProjectID":     projectID,
		"ProjectName":   projectName,
		"PageTitle":     "witness - " + projectName,
		"ActiveProject": projectID,
	})
}

// SessionDetail renders the full session view.
func (h *Handlers) SessionDetail(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	sessionID := chi.URLParam(r, "sessionID")

	// ETag based on session file mtime.
	if h.CheckETag(w, r, h.disc.SessionFileMtime(projectID, sessionID)) {
		return
	}

	session, err := h.disc.GetSession(projectID, sessionID)
	if err != nil {
		log.Printf("get session %s/%s: %v", projectID, sessionID, err)
		if os.IsNotExist(err) {
			h.renderError(w, r, http.StatusNotFound, "Session not found", "Session "+sessionID+" does not exist")
		} else {
			h.renderError(w, r, http.StatusBadRequest, "Bad request", "Could not load session "+sessionID)
		}
		return
	}

	// Attach agent persona if this is an agent session.
	if session.AgentSetting != "" {
		session.AgentPersona = h.disc.GetAgentPersona(session.AgentSetting)
	}

	// Set ViewURLs for subagent links.
	for i := range session.Subagents {
		session.Subagents[i].ViewURL = fmt.Sprintf("/projects/%s/sessions/%s/subagents/%s",
			projectID, sessionID, session.Subagents[i].AgentID)
	}

	// Set plan path.
	session.PlanPath = h.disc.ResolvePlan(session.Slug)

	// Load audit events for this session.
	auditEvents := h.disc.GetAuditForSession(session.ID, session.StartTime, session.EndTime)

	projects, _ := h.disc.ListProjects()

	// Lazy-load: inline first 30 turns, rest loaded on demand.
	inlineTurns := session.Turns
	hasMore := false
	if len(session.Turns) > 30 {
		inlineTurns = session.Turns[:30]
		hasMore = true
	}

	h.render(w, r, "session", map[string]any{
		"Projects":      projects,
		"Session":       session,
		"InlineTurns":   inlineTurns,
		"HasMoreTurns":  hasMore,
		"NextOffset":    30,
		"ProjectID":     projectID,
		"MaxDuration":   session.MaxDuration,
		"PageTitle":     "witness - " + session.Title,
		"ActiveProject": projectID,
		"AuditEvents":   auditEvents,
	})
}

// SubagentDetail renders the session view for a subagent JSONL file.
func (h *Handlers) SubagentDetail(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	sessionID := chi.URLParam(r, "sessionID")
	agentID := chi.URLParam(r, "agentID")

	session, err := h.disc.GetSubagentSession(projectID, sessionID, agentID)
	if err != nil {
		h.renderError(w, r, http.StatusNotFound, "Subagent not found", "Subagent "+agentID+" does not exist")
		return
	}

	if session.AgentSetting != "" {
		session.AgentPersona = h.disc.GetAgentPersona(session.AgentSetting)
	}

	// Extract the task brief from the first user turn (the prompt that spawned this subagent).
	var taskBrief string
	if len(session.Turns) > 0 {
		taskBrief = session.Turns[0].UserText
	}

	projects, _ := h.disc.ListProjects()

	h.render(w, r, "session", map[string]interface{}{
		"Session":         session,
		"ProjectID":       projectID,
		"ParentSessionID": sessionID,
		"IsSubagent":      true,
		"TaskBrief":       taskBrief,
		"AgentID":         agentID,
		"PageTitle":       "witness - agent " + agentID,
		"ActiveProject":   projectID,
		"Projects":        projects,
	})
}

// SessionTurns returns a batch of turn partials for lazy-loading.
func (h *Handlers) SessionTurns(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	sessionID := chi.URLParam(r, "sessionID")

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 30
	}

	session, err := h.disc.GetSession(projectID, sessionID)
	if err != nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "Session "+sessionID+" does not exist")
		return
	}

	if offset >= len(session.Turns) {
		w.WriteHeader(http.StatusOK)
		return
	}

	end := offset + limit
	if end > len(session.Turns) {
		end = len(session.Turns)
	}
	batch := session.Turns[offset:end]

	// Render each turn partial.
	t, ok := h.pages["session"]
	if !ok {
		h.renderError(w, r, http.StatusInternalServerError, "Template error", "session template not found")
		return
	}

	var buf bytes.Buffer
	for _, turn := range batch {
		data := map[string]any{
			"Turn":        turn,
			"MaxDuration": session.MaxDuration,
		}
		if err := t.ExecuteTemplate(&buf, "turn_wrap", data); err != nil {
			log.Printf("turn render error: %v", err)
		}
	}

	// Add sentinel for next batch if there are more turns.
	if end < len(session.Turns) {
		nextOffset := end
		fmt.Fprintf(&buf,
			`<div hx-get="/projects/%s/sessions/%s/turns?offset=%d&limit=%d" hx-trigger="revealed" hx-swap="outerHTML" class="h-8"></div>`,
			projectID, sessionID, nextOffset, limit)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

// SessionPlan renders the plan file for a session.
func (h *Handlers) SessionPlan(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	sessionID := chi.URLParam(r, "sessionID")

	session, err := h.disc.GetSession(projectID, sessionID)
	if err != nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "Session "+sessionID+" does not exist")
		return
	}

	planPath := h.disc.ResolvePlan(session.Slug)
	if planPath == "" {
		h.renderError(w, r, http.StatusNotFound, "No plan found", "No plan file is associated with this session")
		return
	}

	planData, err := os.ReadFile(planPath)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Plan unreadable", "Could not read the plan file")
		return
	}
	content := string(planData)

	projects, _ := h.disc.ListProjects()

	h.render(w, r, "plan", map[string]interface{}{
		"Session":       session,
		"ProjectID":     projectID,
		"PlanContent":   content,
		"PageTitle":     "witness - plan",
		"ActiveProject": projectID,
		"Projects":      projects,
	})
}
