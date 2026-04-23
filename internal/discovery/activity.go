package discovery

import (
	"sync"
	"time"
)

// ActivityWindow is how recently a session must have been written to for it
// to count as "active" in the sidebar indicator. Claude Code writes many
// records per second when a turn is streaming and goes silent between turns;
// 30s keeps the dot visible across short thinking pauses without lingering
// after the agent actually stops.
const ActivityWindow = 30 * time.Second

// ActivityTracker records the most recent write time for each session it has
// seen, and derives per-project activity from those timestamps. It is safe
// for concurrent use.
type ActivityTracker struct {
	mu       sync.RWMutex
	sessions map[string]time.Time // "projectID/sessionID" → last write
	projects map[string]time.Time // projectID → last write across any session
	now      func() time.Time     // overridable for tests
}

// NewActivityTracker returns an empty tracker.
func NewActivityTracker() *ActivityTracker {
	return &ActivityTracker{
		sessions: make(map[string]time.Time),
		projects: make(map[string]time.Time),
		now:      time.Now,
	}
}

// Mark records a write for (projectID, sessionID). sessionID may be empty for
// project-level events — in that case only the project timestamp updates.
func (t *ActivityTracker) Mark(projectID, sessionID string) {
	if projectID == "" {
		return
	}
	now := t.now()
	t.mu.Lock()
	t.projects[projectID] = now
	if sessionID != "" {
		t.sessions[projectID+"/"+sessionID] = now
	}
	t.mu.Unlock()
}

// IsProjectActive reports whether the project has been written to within
// ActivityWindow of now.
func (t *ActivityTracker) IsProjectActive(projectID string) bool {
	t.mu.RLock()
	last := t.projects[projectID]
	t.mu.RUnlock()
	return !last.IsZero() && t.now().Sub(last) <= ActivityWindow
}

// IsSessionActive reports whether the session has been written to within
// ActivityWindow of now.
func (t *ActivityTracker) IsSessionActive(projectID, sessionID string) bool {
	t.mu.RLock()
	last := t.sessions[projectID+"/"+sessionID]
	t.mu.RUnlock()
	return !last.IsZero() && t.now().Sub(last) <= ActivityWindow
}

// ActiveProjects returns the set of projectIDs currently active.
func (t *ActivityTracker) ActiveProjects() map[string]bool {
	cutoff := t.now().Add(-ActivityWindow)
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]bool, len(t.projects))
	for id, last := range t.projects {
		if last.After(cutoff) {
			out[id] = true
		}
	}
	return out
}

// ActiveSessions returns the set of active "projectID/sessionID" keys.
func (t *ActivityTracker) ActiveSessions() map[string]bool {
	cutoff := t.now().Add(-ActivityWindow)
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]bool, len(t.sessions))
	for key, last := range t.sessions {
		if last.After(cutoff) {
			out[key] = true
		}
	}
	return out
}
