package claudelog

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ijiti/witness/internal/costlog"
)

// ParseSessionFile opens and parses a session JSONL file into a Session.
func ParseSessionFile(path, projectID string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	records, err := ParseRecords(f)
	if err != nil {
		return nil, err
	}
	sess, err := BuildSession(records, projectID)
	if err != nil {
		return nil, err
	}

	// Discover subagents from filesystem + records.
	// Session dir is the JSONL filename without extension (e.g., <uuid>/).
	sessionDir := strings.TrimSuffix(path, filepath.Ext(path))
	sess.Subagents = DiscoverSubagents(records, sessionDir)
	sess.HasSubagents = len(sess.Subagents) > 0
	if sess.HasSubagents {
		for _, sa := range sess.Subagents {
			sess.SubagentCost += sa.TotalCost
		}
		LinkSubagentsToToolCalls(sess.Turns, sess.Subagents)
	}

	return sess, nil
}

// ParseSubagentFile parses a subagent JSONL file into a Session.
// Unlike ParseSessionFile, it does not filter out isSidechain records
// (subagent files have isSidechain=true on all records).
func ParseSubagentFile(path, projectID, agentID string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	records, err := ParseRecords(f)
	if err != nil {
		return nil, err
	}
	return buildSubagentSession(records, projectID, agentID)
}

// BuildSession constructs a Session from raw records.
func BuildSession(records []*RawRecord, projectID string) (*Session, error) {
	return buildSessionInternal(records, projectID, true, "")
}

func buildSessionInternal(records []*RawRecord, projectID string, skipSidechain bool, fallbackTitle string) (*Session, error) {
	if len(records) == 0 {
		return &Session{ProjectID: projectID}, nil
	}

	merged := mergeStreamingRecords(records)

	sess := &Session{
		ProjectID: projectID,
	}

	extractSessionMeta(merged, sess)

	sess.Turns = buildTurns(merged, sess, skipSidechain)

	computeSessionTotals(sess)

	if sess.Title == "" {
		if fallbackTitle != "" {
			sess.Title = fallbackTitle
		} else {
			sess.Title = sess.Slug
		}
	}

	return sess, nil
}

// computeSessionTotals fills in token totals, model, max duration, and per-turn/total cost.
func computeSessionTotals(sess *Session) {
	// First pass: aggregate tokens, find max duration, and detect session model.
	for _, t := range sess.Turns {
		sess.TotalInputTokens += t.InputTokens
		sess.TotalOutputTokens += t.OutputTokens
		sess.TotalCacheCreate += t.CacheCreate
		sess.TotalCacheRead += t.CacheRead
		if t.Duration > sess.MaxDuration {
			sess.MaxDuration = t.Duration
		}
		if sess.Model == "" && t.Model != "" {
			sess.Model = t.Model
		}
	}

	// Second pass: compute per-turn cost (needs sess.Model as fallback).
	for i := range sess.Turns {
		t := &sess.Turns[i]
		model := t.Model
		if model == "" {
			model = sess.Model
		}
		t.Cost = costlog.Cost(model, t.InputTokens, t.OutputTokens, t.CacheCreate, t.CacheRead)
		sess.TotalCost += t.Cost
	}
}

// mergeStreamingRecords collapses multiple assistant records sharing the same
// message.id into one record with all content blocks combined.
func mergeStreamingRecords(records []*RawRecord) []*RawRecord {
	result := make([]*RawRecord, 0, len(records))
	var currentGroup []*RawRecord
	var currentMsgID string

	flushGroup := func() {
		if len(currentGroup) == 0 {
			return
		}
		if len(currentGroup) == 1 {
			result = append(result, currentGroup[0])
		} else {
			result = append(result, mergeAssistantGroup(currentGroup))
		}
		currentGroup = nil
		currentMsgID = ""
	}

	for _, r := range records {
		if r.Type == TypeAssistant && r.Message != nil && r.Message.ID != "" && r.Message.ID != "<synthetic>" {
			if r.Message.ID == currentMsgID {
				currentGroup = append(currentGroup, r)
				continue
			}
			flushGroup()
			currentMsgID = r.Message.ID
			currentGroup = []*RawRecord{r}
		} else {
			flushGroup()
			result = append(result, r)
		}
	}
	flushGroup()

	return result
}

// mergeAssistantGroup merges multiple streaming records into one.
func mergeAssistantGroup(group []*RawRecord) *RawRecord {
	merged := *group[0]

	// Each streaming record has exactly 1 content block.
	// Collect all blocks from all records in order.
	var allBlocks []ContentBlock
	for _, r := range group {
		if r.Message == nil {
			continue
		}
		blocks, err := ParseAssistantContent(r.Message.Content)
		if err != nil {
			continue
		}
		allBlocks = append(allBlocks, blocks...)
	}

	// Re-serialize the combined blocks into the merged message.
	last := group[len(group)-1]
	merged.Message = &RawMessage{
		Role:       last.Message.Role,
		ID:         last.Message.ID,
		Model:      last.Message.Model,
		Type:       last.Message.Type,
		StopReason: last.Message.StopReason,
		Usage:      last.Message.Usage,
	}
	if combinedContent, err := json.Marshal(allBlocks); err == nil {
		merged.Message.Content = combinedContent
	}
	merged.UUID = group[0].UUID
	merged.Timestamp = group[0].Timestamp

	// Take usage from the record that has the highest token counts.
	var bestUsage *RawUsage
	for _, r := range group {
		if r.Message != nil && r.Message.Usage != nil {
			u := r.Message.Usage
			if bestUsage == nil || (u.InputTokens+u.OutputTokens+u.CacheCreationInputTokens+u.CacheReadInputTokens) >
				(bestUsage.InputTokens+bestUsage.OutputTokens+bestUsage.CacheCreationInputTokens+bestUsage.CacheReadInputTokens) {
				bestUsage = u
			}
		}
	}
	if bestUsage != nil && merged.Message != nil {
		merged.Message.Usage = bestUsage
	}

	return &merged
}

// buildTurns walks merged records and groups them into conversation turns.
// When skipSidechain is true, sidechain (subagent) records are filtered out.
// When false, all records are included (for parsing subagent JSONL files where
// every record has IsSidechain=true).
func buildTurns(records []*RawRecord, sess *Session, skipSidechain bool) []Turn {
	var turns []Turn

	// Index tool results by their tool_use_id for matching.
	toolResults := make(map[string]*ToolResult, len(records)/4)
	for _, r := range records {
		if r.Type != TypeUser || r.Message == nil || !r.IsMeta {
			continue
		}
		_, blocks, err := ParseUserContent(r.Message.Content)
		if err != nil || len(blocks) == 0 {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_result" && b.ToolUseID != "" {
				content := ExtractToolResultContent(b.Content)
				if len(content) > 8000 {
					content = content[:7997] + "..."
				}
				toolResults[b.ToolUseID] = &ToolResult{
					ToolUseID: b.ToolUseID,
					IsError:   b.IsError,
					Content:   content,
				}
			}
		}
	}

	// Collect turn_duration system events indexed by parentUUID.
	durations := make(map[string]time.Duration, 8)
	for _, r := range records {
		if r.Type == TypeSystem && r.Subtype == SubtypeTurnDuration && r.ParentUUID != nil {
			durations[*r.ParentUUID] = time.Duration(r.DurationMs) * time.Millisecond
		}
	}

	// Count compactions and index their positions.
	compactionBeforeUUID := make(map[string]bool, 4)
	var lastCompactionUUID string
	for _, r := range records {
		if r.Type == TypeSystem && r.Subtype == SubtypeCompactBoundary {
			sess.Compactions++
			lastCompactionUUID = r.UUID
		} else if r.Type == TypeUser && !r.IsMeta && lastCompactionUUID != "" {
			// The first non-meta user record after a compaction boundary.
			compactionBeforeUUID[r.UUID] = true
			lastCompactionUUID = ""
		}
	}

	turnIdx := 0
	for i, r := range records {
		// A turn starts with a non-meta user record.
		if r.Type != TypeUser || r.Message == nil || r.IsMeta {
			continue
		}
		if r.Message.Role != "user" {
			continue
		}
		if skipSidechain && r.IsSidechain {
			continue
		}

		userText, _, _ := ParseUserContent(r.Message.Content)

		turn := Turn{
			Index:            turnIdx,
			Timestamp:        r.Timestamp,
			UserText:         userText,
			IsCompactSummary: r.IsCompactSummary,
			CompactionBefore: compactionBeforeUUID[r.UUID],
		}

		// Find the next assistant record(s) after this user record.
		for j := i + 1; j < len(records); j++ {
			ar := records[j]

			// Stop at the next non-meta user record (next turn).
			if ar.Type == TypeUser && !ar.IsMeta && ar.Message != nil && ar.Message.Role == "user" {
				if !skipSidechain || !ar.IsSidechain {
					break
				}
			}

			if ar.Type != TypeAssistant || ar.Message == nil {
				continue
			}
			if skipSidechain && ar.IsSidechain {
				continue
			}

			blocks, err := ParseAssistantContent(ar.Message.Content)
			if err != nil {
				continue
			}

			if ar.Message.Model != "" && ar.Message.Model != "<synthetic>" {
				turn.Model = ar.Message.Model
			}

			var assistParts []string
			var thinkParts []string
			for _, b := range blocks {
				switch b.Type {
				case "text":
					assistParts = append(assistParts, b.Text)
				case "thinking":
					thinkParts = append(thinkParts, b.Thinking)
				case "tool_use":
					tc := ToolCall{
						ID:    b.ID,
						Name:  b.Name,
						Input: ParseToolInput(b.Name, b.Input),
					}
					if tr, ok := toolResults[b.ID]; ok {
						tc.Result = tr
					}
					turn.ToolCalls = append(turn.ToolCalls, tc)
				}
			}
			if len(assistParts) > 0 {
				if turn.AssistantText != "" {
					turn.AssistantText += "\n"
				}
				turn.AssistantText += strings.Join(assistParts, "\n")
			}
			if len(thinkParts) > 0 {
				if turn.ThinkingText != "" {
					turn.ThinkingText += "\n"
				}
				turn.ThinkingText += strings.Join(thinkParts, "\n")
			}

			// Token usage — take from assistant message.
			if ar.Message.Usage != nil {
				u := ar.Message.Usage
				turn.InputTokens += u.InputTokens
				turn.OutputTokens += u.OutputTokens
				turn.CacheCreate += u.CacheCreationInputTokens
				turn.CacheRead += u.CacheReadInputTokens
			}

			if d, ok := durations[ar.UUID]; ok {
				turn.Duration = d
			}
		}

		turns = append(turns, turn)
		turnIdx++
	}

	return turns
}

// SessionMeta is lightweight metadata extracted without parsing the full file.
type SessionMeta struct {
	ID        string
	Slug      string
	Title     string
	CWD       string
	GitBranch string
	Version   string
	StartTime time.Time
	LineCount int
}

// ExtractMeta reads just enough of a session file to extract metadata.
// Reads at most the first 50 records.
func ExtractMeta(path string) (*SessionMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	records, err := ParseRecords(&limitedReader{r: f, maxRecords: 50})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	meta := &SessionMeta{
		StartTime: records[0].Timestamp,
		LineCount: len(records),
	}

	for _, r := range records {
		if r.SessionID != "" && meta.ID == "" {
			meta.ID = r.SessionID
		}
		if r.Slug != "" && meta.Slug == "" {
			meta.Slug = r.Slug
		}
		if r.CWD != "" && meta.CWD == "" {
			meta.CWD = r.CWD
		}
		if r.GitBranch != "" && meta.GitBranch == "" {
			meta.GitBranch = r.GitBranch
		}
		if r.Version != "" && meta.Version == "" {
			meta.Version = r.Version
		}
		if r.Type == TypeCustomTitle && r.CustomTitle != "" {
			meta.Title = r.CustomTitle
		}

		// Once we have essentials, keep scanning only for custom-title.
	}

	if meta.Title == "" {
		meta.Title = meta.Slug
	}

	return meta, nil
}

// limitedReader wraps an io.Reader and stops after maxRecords JSONL lines.
type limitedReader struct {
	r          io.Reader
	maxRecords int
	count      int
	done       bool
}

func (lr *limitedReader) Read(p []byte) (int, error) {
	if lr.done {
		return 0, io.EOF
	}
	n, err := lr.r.Read(p)
	for i := 0; i < n; i++ {
		if p[i] == '\n' {
			lr.count++
			if lr.count >= lr.maxRecords {
				lr.done = true
				return i + 1, io.EOF
			}
		}
	}
	return n, err
}

// buildSubagentSession constructs a Session from subagent records.
// Passes skipSidechain=false since subagent records all have IsSidechain=true.
func buildSubagentSession(records []*RawRecord, projectID, agentID string) (*Session, error) {
	return buildSessionInternal(records, projectID, false, "agent "+agentID)
}

// extractSessionMeta populates session metadata and settings from merged records.
func extractSessionMeta(merged []*RawRecord, sess *Session) {
	for _, r := range merged {
		if r.SessionID != "" {
			sess.ID = r.SessionID
		}
		if r.Slug != "" {
			sess.Slug = r.Slug
		}
		if r.CWD != "" && sess.CWD == "" {
			sess.CWD = r.CWD
		}
		if r.GitBranch != "" && sess.GitBranch == "" {
			sess.GitBranch = r.GitBranch
		}
		if r.Version != "" && sess.Version == "" {
			sess.Version = r.Version
		}
		if sess.ID != "" && sess.CWD != "" {
			break
		}
	}

	for _, r := range merged {
		if r.Type == TypeCustomTitle && r.CustomTitle != "" {
			sess.Title = r.CustomTitle
		}
		if r.Type == TypeAgentSetting && r.AgentSetting != "" {
			sess.AgentSetting = r.AgentSetting
		}
	}

	sess.StartTime = merged[0].Timestamp
	sess.EndTime = merged[len(merged)-1].Timestamp
}

