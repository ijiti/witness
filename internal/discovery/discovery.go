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

// ProjectGroup nests worktree-style child projects under their filesystem
// parent so the sidebar can fold them away. Children are projects whose CWD
// is a strict subpath of another project's CWD; ungrouped projects appear as
// groups with no children.
type ProjectGroup struct {
	Project           // the top-level (parent) project
	Children []Project
}

// HasChildren reports whether the group has nested worktrees.
func (g ProjectGroup) HasChildren() bool { return len(g.Children) > 0 }

// TotalSessions sums sessions across the group (parent + all children).
func (g ProjectGroup) TotalSessions() int {
	n := g.SessionCount
	for _, c := range g.Children {
		n += c.SessionCount
	}
	return n
}

// GroupProjects nests worktree-style projects under their logical parent so
// the sidebar can fold them away. Two heuristics, in precedence order:
//
//  1. **Path-prefix nesting** — a project whose CWD is a strict descendant of
//     another project's CWD (e.g. `/repo/.git/worktrees/branch` under `/repo`).
//
//  2. **Sibling-worktree convention** — a project whose path contains a
//     `/worktrees/` (or `/.git/worktrees/`) segment AND whose basename starts
//     with `<otherProject>-` is nested under that other project. Catches the
//     common `~/worktrees/<repo>-<branch>` and `~/<repo>-worktrees/<branch>`
//     layouts.
//
// Ungrouped projects appear as groups with no children. Top-level groups
// preserve the input slice's activity ordering; children are sorted most-recent
// first within each group.
func GroupProjects(projects []Project) []ProjectGroup {
	if len(projects) == 0 {
		return nil
	}

	originalIdx := make(map[string]int, len(projects))
	for i, p := range projects {
		originalIdx[p.ID] = i
	}

	// Resolve each project's parent CWD (empty == top-level).
	parentOf := make(map[string]string, len(projects))
	for _, p := range projects {
		parentOf[p.CWD] = findParent(p, projects)
	}

	// A worktree's parent might itself be a worktree (rare). Walk up to the
	// root parent so we never produce two-deep nesting in the sidebar — flatten
	// into a single layer of children under the topmost ancestor.
	for cwd := range parentOf {
		seen := map[string]bool{cwd: true}
		for {
			next, ok := parentOf[parentOf[cwd]]
			if !ok || next == "" || seen[next] {
				break
			}
			seen[parentOf[cwd]] = true
			parentOf[cwd] = parentOf[parentOf[cwd]]
		}
	}

	type indexed struct {
		Project
		idx int
	}
	var parents []indexed
	children := make(map[string][]Project)

	for _, p := range projects {
		if pcwd := parentOf[p.CWD]; pcwd != "" {
			children[pcwd] = append(children[pcwd], p)
		} else {
			parents = append(parents, indexed{Project: p, idx: originalIdx[p.ID]})
		}
	}

	// Top-level: preserve activity order.
	sort.SliceStable(parents, func(i, j int) bool { return parents[i].idx < parents[j].idx })
	// Children: most-recent-active first within each group.
	for cwd, kids := range children {
		sort.SliceStable(kids, func(i, j int) bool { return kids[i].LastActive.After(kids[j].LastActive) })
		children[cwd] = kids
	}

	out := make([]ProjectGroup, len(parents))
	for i, p := range parents {
		out[i] = ProjectGroup{Project: p.Project, Children: children[p.CWD]}
	}
	return out
}

// findParent returns p's logical parent CWD, or "" if p is top-level.
func findParent(p Project, all []Project) string {
	// Heuristic 1: longest strict path prefix.
	var bestPrefix string
	for _, other := range all {
		if other.CWD == p.CWD {
			continue
		}
		if isPathChild(p.CWD, other.CWD) && len(other.CWD) > len(bestPrefix) {
			bestPrefix = other.CWD
		}
	}
	if bestPrefix != "" {
		return bestPrefix
	}

	// Heuristic 2: sibling-worktree naming. Only applies if p's path contains
	// a worktrees segment, to avoid grouping unrelated projects with similar
	// basenames (e.g. `mytool` and `mytool-experimental` are NOT a worktree pair).
	if !looksLikeWorktreePath(p.CWD) {
		return ""
	}
	myBase := filepath.Base(p.CWD)
	// Prefer the longest matching parent name (`mytool-frontend-task-x` should
	// nest under `mytool-frontend`, not `mytool`).
	var bestParentCWD string
	bestBaseLen := 0
	for _, other := range all {
		if other.CWD == p.CWD {
			continue
		}
		otherBase := filepath.Base(other.CWD)
		if otherBase == "" || otherBase == "/" {
			continue
		}
		if strings.HasPrefix(myBase, otherBase+"-") && len(otherBase) > bestBaseLen {
			bestBaseLen = len(otherBase)
			bestParentCWD = other.CWD
		}
	}
	return bestParentCWD
}

// isPathChild reports whether childCWD is a strict descendant of parentCWD.
func isPathChild(childCWD, parentCWD string) bool {
	if parentCWD == "" || parentCWD == "/" || childCWD == parentCWD {
		return false
	}
	if !strings.HasPrefix(childCWD, parentCWD) {
		return false
	}
	rest := childCWD[len(parentCWD):]
	return len(rest) > 0 && (rest[0] == '/' || rest[0] == '\\')
}

// looksLikeWorktreePath returns true for paths that look like git/branch
// worktrees by convention. Used to gate the sibling-naming heuristic.
func looksLikeWorktreePath(cwd string) bool {
	return strings.Contains(cwd, "/worktrees/") ||
		strings.Contains(cwd, "/.git/worktrees/") ||
		strings.Contains(cwd, "\\worktrees\\") ||
		strings.Contains(cwd, "\\.git\\worktrees\\")
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

	// In-memory fresh stats computed from actual JSONL files.
	freshStats     *claudelog.StatsCache
	freshStatsMu   sync.RWMutex
	freshStatsTime time.Time // wall time when freshStats was last computed

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

// statsRecomputeMinInterval is the minimum time between watcher-triggered
// fresh-stats recomputes. Prevents recomputing 50 times during an active Claude
// session that is writing many JSONL records per second.
const statsRecomputeMinInterval = 60 * time.Second

// broadcastDebounce coalesces rapid writes to the same session into a single
// SSE broadcast. Claude Code writes one JSONL record per streaming content
// block, so a single assistant response can fire 30+ inotify events in a few
// seconds; without debouncing each fires a full session re-parse + render in
// every connected SSE client. 200ms feels live (~5 fps) without thrashing.
const broadcastDebounce = 200 * time.Millisecond

// processWatchEvents handles incoming inotify events. Cache invalidation runs
// immediately so the next request sees fresh data; the SSE broadcast is
// debounced per-session so streaming-heavy turns produce one broadcast each
// (rather than 30+).
func (d *Discoverer) processWatchEvents() {
	var lastRecompute time.Time
	var recomputePending bool

	// Per-key (projectID/sessionID) coalescing timers. Latest event wins —
	// when the timer fires we broadcast the most recent event for that key.
	var dbMu sync.Mutex
	timers := make(map[string]*time.Timer)
	pending := make(map[string]WatchEvent)

	for ev := range d.watcher.Events() {
		// Invalidate cache for modified sessions — must NOT be debounced so
		// the next request sees fresh data.
		if ev.Type == "modify" && ev.SessionID != "" {
			cacheKey := ev.ProjectID + "/" + ev.SessionID
			d.mu.Lock()
			delete(d.cache, cacheKey)
			d.mu.Unlock()

			// Schedule a debounced fresh-stats recompute.
			if !recomputePending && time.Since(lastRecompute) >= statsRecomputeMinInterval {
				recomputePending = true
				go func() {
					d.ComputeFreshStats() //nolint:errcheck — errors already logged inside
					lastRecompute = time.Now()
					recomputePending = false
				}()
			}
		}

		// Coalesce broadcasts: one per session per debounce window.
		key := ev.ProjectID + "/" + ev.SessionID
		dbMu.Lock()
		pending[key] = ev
		if t, ok := timers[key]; ok {
			t.Stop()
		}
		timers[key] = time.AfterFunc(broadcastDebounce, func() {
			dbMu.Lock()
			latest, ok := pending[key]
			delete(pending, key)
			delete(timers, key)
			dbMu.Unlock()
			if ok {
				d.Broadcaster.Send(latest)
			}
		})
		dbMu.Unlock()
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

// ComputeFreshStats walks all JSONL session files and builds a fresh StatsCache
// in memory. It logs the duration and stores the result so GetFreshStats can
// return it. Safe to call from multiple goroutines; only one compute runs at a
// time (protected by freshStatsMu). This method is also the target for debounced
// recompute after file-watcher events.
func (d *Discoverer) ComputeFreshStats() error {
	start := time.Now()
	sc, err := claudelog.ComputeStats(d.BaseDir)
	elapsed := time.Since(start)
	if err != nil {
		log.Printf("warning: fresh stats compute error (partial result returned): %v", err)
	}
	if sc != nil {
		d.freshStatsMu.Lock()
		d.freshStats = sc
		d.freshStatsTime = time.Now()
		d.freshStatsMu.Unlock()
		log.Printf("fresh stats computed in %s: %d sessions, %d messages, %.2f total cost",
			elapsed.Round(time.Millisecond),
			sc.TotalSessions, sc.TotalMessages, sc.TotalCost)
	}
	return err
}

// GetFreshStats returns the in-memory fresh StatsCache. Returns nil if
// ComputeFreshStats has not yet completed (e.g., still running in background
// on startup — callers should fall back to GetStats in that case).
func (d *Discoverer) GetFreshStats() *claudelog.StatsCache {
	d.freshStatsMu.RLock()
	defer d.freshStatsMu.RUnlock()
	return d.freshStats
}

// FreshStatsTime returns the wall time when the fresh stats were last computed.
// Returns zero time if no compute has completed.
func (d *Discoverer) FreshStatsTime() time.Time {
	d.freshStatsMu.RLock()
	defer d.freshStatsMu.RUnlock()
	return d.freshStatsTime
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
