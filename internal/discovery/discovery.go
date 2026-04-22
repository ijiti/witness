// Package discovery walks ~/.claude/projects/ to enumerate projects and sessions.
package discovery

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ijiti/witness/internal/claudelog"
)

// validatePathComponent rejects path traversal attempts in URL parameters.
func validatePathComponent(s string) error {
	if s == "" {
		return fmt.Errorf("empty path component")
	}
	if strings.Contains(s, "/") || strings.Contains(s, "\\") || strings.Contains(s, "..") {
		return fmt.Errorf("invalid path component: %q", s)
	}
	if s != filepath.Base(s) {
		return fmt.Errorf("invalid path component: %q", s)
	}
	return nil
}

// Project represents a project directory under ~/.claude/projects/.
type Project struct {
	ID           string // directory name, e.g., "-home-alice-myapp"
	CWD          string // decoded working directory path
	DisplayName  string // basename for UI
	SessionCount int
	LastActive   time.Time
}

// SessionEntry is a discovered session file.
type SessionEntry struct {
	ID      string
	Slug    string
	Title   string
	Branch  string
	Version string
	ModTime time.Time
	Size    int64
	Path    string
}

// Discoverer walks the Claude projects directory.
type Discoverer struct {
	BaseDir   string
	claudeDir string // parent of BaseDir, i.e. ~/.claude/

	mu    sync.RWMutex
	cache map[string]*cacheEntry

	history     map[string]*claudelog.SessionHistory
	historyOnce sync.Once

	agents     map[string]*claudelog.AgentPersona
	agentsOnce sync.Once

	allHistory     []claudelog.HistoryEntry
	allHistoryMu   sync.RWMutex
	allHistoryTime time.Time // mtime of history.jsonl at last load

	stats     *claudelog.StatsCache
	statsMu   sync.RWMutex
	statsTime time.Time // mtime of stats-cache.json at last load

	// Live monitoring (session 3).
	watcher     *Watcher
	Broadcaster *Broadcaster
}

type cacheEntry struct {
	session *claudelog.Session
	modTime time.Time
}

// NewDiscoverer creates a Discoverer for the given base directory.
func NewDiscoverer(baseDir string) *Discoverer {
	return &Discoverer{
		BaseDir:     baseDir,
		claudeDir:   filepath.Dir(baseDir), // ~/.claude/projects → ~/.claude
		cache:       make(map[string]*cacheEntry),
		Broadcaster: NewBroadcaster(),
	}
}

// StartWatching initializes the inotify watcher and begins monitoring.
// On file changes, it invalidates the session cache and broadcasts events.
func (d *Discoverer) StartWatching() error {
	w, err := NewWatcher(d.BaseDir)
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	d.watcher = w

	if err := w.Start(); err != nil {
		return fmt.Errorf("start watcher: %w", err)
	}

	go d.processWatchEvents()
	log.Printf("file watcher started for %s", d.BaseDir)
	return nil
}

// StopWatching shuts down the file watcher.
func (d *Discoverer) StopWatching() {
	if d.watcher != nil {
		d.watcher.Stop()
	}
}

// processWatchEvents handles incoming inotify events.
func (d *Discoverer) processWatchEvents() {
	for ev := range d.watcher.Events() {
		// Invalidate cache for modified sessions.
		if ev.Type == "modify" && ev.SessionID != "" {
			cacheKey := ev.ProjectID + "/" + ev.SessionID
			d.mu.Lock()
			delete(d.cache, cacheKey)
			d.mu.Unlock()
		}

		// Broadcast to all SSE clients.
		d.Broadcaster.Send(ev)
	}
}

func (d *Discoverer) loadHistory() {
	d.historyOnce.Do(func() {
		path := filepath.Join(d.claudeDir, "history.jsonl")
		h, err := claudelog.ParseHistoryFile(path)
		if err != nil {
			log.Printf("warning: could not load history.jsonl: %v", err)
			d.history = make(map[string]*claudelog.SessionHistory)
			return
		}
		d.history = h
		log.Printf("loaded %d session entries from history.jsonl", len(h))
	})
}

func (d *Discoverer) loadAgents() {
	d.agentsOnce.Do(func() {
		dir := filepath.Join(d.claudeDir, "agents")
		a, err := claudelog.ParseAgentPersonas(dir)
		if err != nil {
			log.Printf("warning: could not load agent personas: %v", err)
			d.agents = make(map[string]*claudelog.AgentPersona)
			return
		}
		d.agents = a
		log.Printf("loaded %d agent personas", len(a))
	})
}

// GetAgentPersona returns the persona for a given agent setting name, or nil.
func (d *Discoverer) GetAgentPersona(name string) *claudelog.AgentPersona {
	d.loadAgents()
	if p, ok := d.agents[name]; ok {
		return p
	}
	// AgentSetting might be a full path — strip to basename.
	base := filepath.Base(name)
	base = strings.TrimSuffix(base, ".md")
	if p, ok := d.agents[base]; ok {
		return p
	}
	return nil
}

// truncateTitle caps a title string at 120 characters with an ellipsis.
func truncateTitle(s string) string {
	if len(s) > 120 {
		return s[:117] + "..."
	}
	return s
}

// EnrichTitle sets the session title from history.jsonl if the current title
// is just the slug or empty.
func (d *Discoverer) EnrichTitle(sess *claudelog.Session) {
	d.loadHistory()
	if sess.Title != "" && sess.Title != sess.Slug {
		return
	}
	h, ok := d.history[sess.ID]
	if !ok || h.FirstPrompt == "" {
		return
	}
	sess.Title = truncateTitle(h.FirstPrompt)
}

// ListProjects returns all projects sorted by most recent session activity.
func (d *Discoverer) ListProjects() ([]Project, error) {
	entries, err := os.ReadDir(d.BaseDir)
	if err != nil {
		return nil, err
	}

	// Phase 1: enumerate projects sequentially (fast dir ops).
	type candidate struct {
		dirName      string
		sessionCount int
		lastActive   time.Time
		firstPath    string // path of most recent session for ExtractMeta
	}
	var candidates []candidate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirName := e.Name()
		sessDir := filepath.Join(d.BaseDir, dirName)
		sessions, err := listJSONLFiles(sessDir)
		if err != nil || len(sessions) == 0 {
			continue
		}
		var lastActive time.Time
		for _, s := range sessions {
			if s.ModTime.After(lastActive) {
				lastActive = s.ModTime
			}
		}
		candidates = append(candidates, candidate{
			dirName:      dirName,
			sessionCount: len(sessions),
			lastActive:   lastActive,
			firstPath:    sessions[0].Path,
		})
	}

	// Phase 2: enrich CWD via ExtractMeta in parallel (bounded to 8 goroutines).
	projects := make([]Project, len(candidates))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for idx, c := range candidates {
		cwd := decodeDirName(c.dirName)
		projects[idx] = Project{
			ID:           c.dirName,
			CWD:          cwd,
			DisplayName:  filepath.Base(cwd),
			SessionCount: c.sessionCount,
			LastActive:   c.lastActive,
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, path string) {
			defer wg.Done()
			defer func() { <-sem }()
			meta, err := claudelog.ExtractMeta(path)
			if err == nil && meta != nil && meta.CWD != "" {
				projects[i].CWD = meta.CWD
				projects[i].DisplayName = filepath.Base(meta.CWD)
			}
		}(idx, c.firstPath)
	}
	wg.Wait()

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastActive.After(projects[j].LastActive)
	})

	return projects, nil
}

// ListSessions returns session entries for a project, sorted by mtime desc.
func (d *Discoverer) ListSessions(projectID string) ([]SessionEntry, error) {
	if err := validatePathComponent(projectID); err != nil {
		return nil, err
	}
	sessDir := filepath.Join(d.BaseDir, projectID)
	files, err := listJSONLFiles(sessDir)
	if err != nil {
		return nil, err
	}

	// Enrich with metadata from each file in parallel (bounded to 8 goroutines).
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			meta, err := claudelog.ExtractMeta(files[idx].Path)
			if err != nil || meta == nil {
				return
			}
			if meta.ID != "" {
				files[idx].ID = meta.ID
			}
			files[idx].Slug = meta.Slug
			files[idx].Title = meta.Title
			files[idx].Branch = meta.GitBranch
			files[idx].Version = meta.Version
		}(i)
	}
	wg.Wait()

	// Enrich titles from history.jsonl (use first user prompt when no custom title).
	d.loadHistory()
	for i := range files {
		h, ok := d.history[files[i].ID]
		if !ok {
			continue
		}
		// Only override if title is still the slug or empty.
		if files[i].Title == files[i].Slug || files[i].Title == "" {
			if h.FirstPrompt != "" {
				files[i].Title = truncateTitle(h.FirstPrompt)
			}
		}
	}

	return files, nil
}

// GetSession parses a full session, using cache if the file hasn't changed.
func (d *Discoverer) GetSession(projectID, sessionID string) (*claudelog.Session, error) {
	if err := validatePathComponent(projectID); err != nil {
		return nil, err
	}
	if err := validatePathComponent(sessionID); err != nil {
		return nil, err
	}
	sessDir := filepath.Join(d.BaseDir, projectID)
	path := filepath.Join(sessDir, sessionID+".jsonl")

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	cacheKey := projectID + "/" + sessionID
	d.mu.RLock()
	if ce, ok := d.cache[cacheKey]; ok && ce.modTime.Equal(info.ModTime()) {
		d.mu.RUnlock()
		return ce.session, nil
	}
	d.mu.RUnlock()

	sess, err := claudelog.ParseSessionFile(path, projectID)
	if err != nil {
		return nil, err
	}

	d.mu.Lock()
	d.cache[cacheKey] = &cacheEntry{session: sess, modTime: info.ModTime()}
	d.mu.Unlock()

	return sess, nil
}

// GetSubagentSession parses a subagent JSONL file and returns a Session.
func (d *Discoverer) GetSubagentSession(projectID, sessionID, agentID string) (*claudelog.Session, error) {
	if err := validatePathComponent(projectID); err != nil {
		return nil, err
	}
	if err := validatePathComponent(sessionID); err != nil {
		return nil, err
	}
	if err := validatePathComponent(agentID); err != nil {
		return nil, err
	}

	path := filepath.Join(d.BaseDir, projectID, sessionID, "subagents", "agent-"+agentID+".jsonl")
	return claudelog.ParseSubagentFile(path, projectID, agentID)
}

// ResolvePlan checks if a plan file exists matching the session slug.
func (d *Discoverer) ResolvePlan(slug string) string {
	if slug == "" {
		return ""
	}
	plansDir := filepath.Join(d.claudeDir, "plans")
	path := filepath.Join(plansDir, slug+".md")
	// Guard against path traversal — slug comes from session JSONL (agent-writable).
	if !strings.HasPrefix(path, plansDir+string(filepath.Separator)) {
		return ""
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func (d *Discoverer) loadAllHistory() {
	path := filepath.Join(d.claudeDir, "history.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	d.allHistoryMu.RLock()
	cached := d.allHistoryTime.Equal(info.ModTime())
	d.allHistoryMu.RUnlock()
	if cached {
		return
	}

	h, err := claudelog.ParseAllHistory(path)
	if err != nil {
		log.Printf("warning: could not load all history: %v", err)
		return
	}

	d.allHistoryMu.Lock()
	d.allHistory = h
	d.allHistoryTime = info.ModTime()
	d.allHistoryMu.Unlock()
	log.Printf("loaded %d history entries for search", len(h))
}

// SearchSessions performs a case-insensitive substring search across all history entries.
// Returns matching results sliced by offset/limit and the total match count.
func (d *Discoverer) SearchSessions(query string, offset, limit int) ([]claudelog.SearchResult, int, error) {
	d.loadAllHistory()
	d.allHistoryMu.RLock()
	entries := d.allHistory
	d.allHistoryMu.RUnlock()
	results, total := claudelog.SearchHistory(entries, query, offset, limit)
	return results, total, nil
}

// GetStats returns the parsed stats-cache.json, reloading if the file changed.
func (d *Discoverer) GetStats() *claudelog.StatsCache {
	path := filepath.Join(d.claudeDir, "stats-cache.json")
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}

	d.statsMu.RLock()
	cached := d.stats != nil && d.statsTime.Equal(info.ModTime())
	d.statsMu.RUnlock()
	if cached {
		return d.stats
	}

	sc, err := claudelog.ParseStatsCache(path)
	if err != nil {
		log.Printf("warning: could not load stats-cache.json: %v", err)
		return nil
	}

	d.statsMu.Lock()
	d.stats = sc
	d.statsTime = info.ModTime()
	d.statsMu.Unlock()
	log.Printf("loaded stats-cache.json (last computed: %s)", sc.LastComputedDate)

	return sc
}

// StatsFileMtime returns the mtime of stats-cache.json (for ETag).
func (d *Discoverer) StatsFileMtime() time.Time {
	path := filepath.Join(d.claudeDir, "stats-cache.json")
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// SessionFileMtime returns the mtime of a session JSONL file (for ETag).
func (d *Discoverer) SessionFileMtime(projectID, sessionID string) time.Time {
	if validatePathComponent(projectID) != nil || validatePathComponent(sessionID) != nil {
		return time.Time{}
	}
	path := filepath.Join(d.BaseDir, projectID, sessionID+".jsonl")
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// GetAuditForSession loads audit events for a session from the audit directory.
func (d *Discoverer) GetAuditForSession(sessionID string, startTime, endTime time.Time) []claudelog.AuditEvent {
	auditDir := filepath.Join(d.claudeDir, "audit")
	return claudelog.LoadAuditForSession(auditDir, sessionID, startTime, endTime)
}

// listJSONLFiles returns .jsonl files in a directory, sorted by mtime desc.
func listJSONLFiles(dir string) ([]SessionEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []SessionEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		// Skip very small files (likely empty or metadata-only).
		if info.Size() < 100 {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		files = append(files, SessionEntry{
			ID:      id,
			Path:    filepath.Join(dir, e.Name()),
			ModTime: info.ModTime(),
			Size:    info.Size(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})

	return files, nil
}

// decodeDirName converts a Claude projects directory name to a filesystem path.
// Claude encodes paths by replacing / with - and prepending -.
// This is ambiguous for paths containing hyphens, so this is a best-effort decode.
func decodeDirName(name string) string {
	if name == "" {
		return "/"
	}
	return strings.ReplaceAll(name, "-", "/")
}
