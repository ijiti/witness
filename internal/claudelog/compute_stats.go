package claudelog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ijiti/witness/internal/costlog"
)

// ComputeStats walks all session JSONL files under claudeProjectsDir and
// builds a fresh StatsCache from the actual data. It never reads or writes
// stats-cache.json. On error it returns a partial result and the first error
// encountered; callers that want a best-effort result should use the returned
// cache even when err != nil.
func ComputeStats(claudeProjectsDir string) (*StatsCache, error) {
	// Enumerate project directories.
	entries, err := os.ReadDir(claudeProjectsDir)
	if err != nil {
		return nil, fmt.Errorf("compute stats: read projects dir: %w", err)
	}

	// Collect all JSONL paths (project-dir + path).
	type sessionPath struct {
		projectID string
		path      string
	}
	var paths []sessionPath

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		projID := e.Name()
		projDir := filepath.Join(claudeProjectsDir, projID)

		// Top-level sessions.
		files, err := os.ReadDir(projDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			info, err := f.Info()
			if err != nil || info.Size() < 100 {
				continue
			}
			paths = append(paths, sessionPath{
				projectID: projID,
				path:      filepath.Join(projDir, f.Name()),
			})

			// Subagent sessions live under <session-uuid>/subagents/.
			sessionBase := strings.TrimSuffix(f.Name(), ".jsonl")
			subagentDir := filepath.Join(projDir, sessionBase, "subagents")
			subFiles, err := os.ReadDir(subagentDir)
			if err != nil {
				continue // no subagents for this session
			}
			for _, sf := range subFiles {
				if sf.IsDir() || !strings.HasSuffix(sf.Name(), ".jsonl") {
					continue
				}
				si, err := sf.Info()
				if err != nil || si.Size() < 100 {
					continue
				}
				paths = append(paths, sessionPath{
					projectID: projID,
					path:      filepath.Join(subagentDir, sf.Name()),
				})
			}
		}
	}

	// Parse all files in parallel, bounded to 8 goroutines.
	type result struct {
		sess *Session
		err  error
	}
	results := make([]result, len(paths))

	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)

	for i, sp := range paths {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, p sessionPath) {
			defer wg.Done()
			defer func() { <-sem }()
			sess, err := ParseSessionFile(p.path, p.projectID)
			results[idx] = result{sess: sess, err: err}
		}(i, sp)
	}
	wg.Wait()

	// Accumulate into StatsCache.
	sc := &StatsCache{
		ModelUsage: make(map[string]ModelUsageStats),
		HourCounts: make(map[string]int),
	}

	// Per-date accumulators (built as maps, converted to slices at the end).
	type dayActivity struct {
		messageCount  int
		sessionCount  int
		toolCallCount int
	}
	dailyAct := make(map[string]*dayActivity)
	// Per-date, per-model token sums.
	type dayModel = map[string]int
	dailyTokens := make(map[string]dayModel)

	var firstError error
	var longestDurationMs int64

	for _, r := range results {
		if r.err != nil {
			if firstError == nil {
				firstError = r.err
			}
			continue
		}
		sess := r.sess
		if sess == nil {
			continue
		}

		sc.TotalSessions++

		// Session start date (UTC).
		if !sess.StartTime.IsZero() {
			dateStr := sess.StartTime.UTC().Format("2006-01-02")
			if sc.FirstSessionDate == "" || dateStr < sc.FirstSessionDate {
				sc.FirstSessionDate = dateStr
			}

			if dailyAct[dateStr] == nil {
				dailyAct[dateStr] = &dayActivity{}
			}
			dailyAct[dateStr].sessionCount++
		}

		// Per-turn aggregation.
		for _, t := range sess.Turns {
			sc.TotalMessages++

			dateStr := t.Timestamp.UTC().Format("2006-01-02")
			hourStr := fmt.Sprintf("%d", t.Timestamp.UTC().Hour())

			// Daily activity.
			if dailyAct[dateStr] == nil {
				dailyAct[dateStr] = &dayActivity{}
			}
			dailyAct[dateStr].messageCount++
			dailyAct[dateStr].toolCallCount += len(t.ToolCalls)

			// Hour counts.
			sc.HourCounts[hourStr]++

			// Model token accumulation.
			model := t.Model
			if model == "" {
				model = sess.Model
			}
			if model != "" {
				mu := sc.ModelUsage[model]
				mu.InputTokens += t.InputTokens
				mu.OutputTokens += t.OutputTokens
				mu.CacheCreationInputTokens += t.CacheCreate
				mu.CacheReadInputTokens += t.CacheRead
				sc.ModelUsage[model] = mu

				// Daily model tokens.
				if dailyTokens[dateStr] == nil {
					dailyTokens[dateStr] = make(dayModel)
				}
				dailyTokens[dateStr][model] += t.InputTokens + t.OutputTokens
			}
		}

		// Longest session by duration.
		durationMs := sess.MaxDuration.Milliseconds()
		if durationMs > longestDurationMs {
			longestDurationMs = durationMs
			sc.LongestSession = LongestSession{
				SessionID:    sess.ID,
				Duration:     durationMs,
				MessageCount: len(sess.Turns),
				Timestamp:    sess.StartTime.UTC().Format(time.RFC3339),
			}
		}
	}

	// Compute CostUSD per model and total cost.
	for model, mu := range sc.ModelUsage {
		mu.CostUSD = costlog.Cost(model,
			mu.InputTokens, mu.OutputTokens,
			mu.CacheCreationInputTokens, mu.CacheReadInputTokens)
		sc.ModelUsage[model] = mu
		sc.TotalCost += mu.CostUSD
	}

	// Convert daily maps to sorted slices.
	for date, da := range dailyAct {
		sc.DailyActivity = append(sc.DailyActivity, DailyActivity{
			Date:          date,
			MessageCount:  da.messageCount,
			SessionCount:  da.sessionCount,
			ToolCallCount: da.toolCallCount,
		})
	}
	sort.Slice(sc.DailyActivity, func(i, j int) bool {
		return sc.DailyActivity[i].Date < sc.DailyActivity[j].Date
	})

	for date, models := range dailyTokens {
		sc.DailyModelTokens = append(sc.DailyModelTokens, DailyTokens{
			Date:          date,
			TokensByModel: models,
		})
	}
	sort.Slice(sc.DailyModelTokens, func(i, j int) bool {
		return sc.DailyModelTokens[i].Date < sc.DailyModelTokens[j].Date
	})

	sc.LastComputedDate = time.Now().UTC().Format("2006-01-02")

	return sc, firstError
}
