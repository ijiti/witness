package claudelog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/ijiti/witness/internal/costlog"
)

// toolUseResultAgent is the typed parse of toolUseResult when it contains agentId.
type toolUseResultAgent struct {
	Status            string    `json:"status"`
	AgentID           string    `json:"agentId"`
	TotalDurationMs   int64     `json:"totalDurationMs"`
	TotalTokens       int       `json:"totalTokens"`
	TotalToolUseCount int       `json:"totalToolUseCount"`
	Usage             *RawUsage `json:"usage"`
}

// DiscoverSubagents scans a session's records and filesystem to build SubagentInfo.
// It performs a three-way merge:
//  1. Task tool_use blocks → map[toolUseID]ToolInputTask
//  2. Progress records with agent_progress → map[agentID]parentToolUseID
//  3. User records with toolUseResult.agentId → completion data
//  4. Filesystem: <sessionDir>/subagents/agent-*.jsonl
func DiscoverSubagents(records []*RawRecord, sessionDir string) []SubagentInfo {
	// Step 1: Find Task tool_use calls.
	taskCalls := make(map[string]ToolInputTask) // toolUseID → input
	for _, r := range records {
		if r.Type != TypeAssistant || r.Message == nil || r.IsSidechain {
			continue
		}
		blocks, err := ParseAssistantContent(r.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" && b.Name == "Task" {
				var input ToolInputTask
				if err := json.Unmarshal(b.Input, &input); err == nil {
					taskCalls[b.ID] = input
				}
			}
		}
	}

	// Step 2: Map agentID → parentToolUseID from progress records.
	agentToToolUse := make(map[string]string) // agentID → parentToolUseID
	for _, r := range records {
		if r.Type != TypeProgress || r.IsSidechain {
			continue
		}
		pd, err := ParseProgressData(r.Data)
		if err != nil || pd == nil || pd.Type != "agent_progress" {
			continue
		}
		if pd.AgentID != "" && r.ParentToolUseID != "" {
			if _, exists := agentToToolUse[pd.AgentID]; !exists {
				agentToToolUse[pd.AgentID] = r.ParentToolUseID
			}
		}
	}

	// Step 3: Parse toolUseResult for completion data.
	agentResults := make(map[string]*toolUseResultAgent) // agentID → result
	for _, r := range records {
		if r.Type != TypeUser || r.IsSidechain || len(r.ToolUseResult) == 0 {
			continue
		}
		var result toolUseResultAgent
		if err := json.Unmarshal(r.ToolUseResult, &result); err == nil && result.AgentID != "" {
			agentResults[result.AgentID] = &result
		}
	}

	// Step 4: Discover subagent files on disk.
	subagentDir := filepath.Join(sessionDir, "subagents")
	entries, err := os.ReadDir(subagentDir)
	if err != nil {
		// No subagents directory — check if we have any from records alone.
		return buildFromRecords(taskCalls, agentToToolUse, agentResults)
	}

	var subagents []SubagentInfo
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		// Skip compacted subagent files.
		if strings.HasPrefix(name, "agent-acompact-") {
			continue
		}
		agentID := strings.TrimPrefix(name, "agent-")
		agentID = strings.TrimSuffix(agentID, ".jsonl")

		info := SubagentInfo{
			AgentID:  agentID,
			FilePath: filepath.Join(subagentDir, name),
		}

		// Link via progress records.
		if toolUseID, ok := agentToToolUse[agentID]; ok {
			info.ParentToolUseID = toolUseID
			if task, ok := taskCalls[toolUseID]; ok {
				info.Description = task.Description
				info.SubagentType = task.SubagentType
				info.Model = task.Model
			}
		}

		// Link via completion results.
		if result, ok := agentResults[agentID]; ok {
			info.Status = result.Status
			info.TotalDurationMs = result.TotalDurationMs
			info.TotalTokens = result.TotalTokens
			info.TotalToolUseCount = result.TotalToolUseCount
			if result.Usage != nil {
				info.TotalCost = turnCostFromUsage(info.Model, result.Usage)
			}
		}

		subagents = append(subagents, info)
	}

	return subagents
}

// buildFromRecords constructs SubagentInfo entries when no subagent directory exists
// but progress/result records reference agents.
func buildFromRecords(taskCalls map[string]ToolInputTask, agentToToolUse map[string]string, agentResults map[string]*toolUseResultAgent) []SubagentInfo {
	if len(agentResults) == 0 && len(agentToToolUse) == 0 {
		return nil
	}

	// Merge all known agent IDs.
	allAgents := make(map[string]bool)
	for id := range agentToToolUse {
		allAgents[id] = true
	}
	for id := range agentResults {
		allAgents[id] = true
	}

	var subagents []SubagentInfo
	for agentID := range allAgents {
		info := SubagentInfo{AgentID: agentID}
		if toolUseID, ok := agentToToolUse[agentID]; ok {
			info.ParentToolUseID = toolUseID
			if task, ok := taskCalls[toolUseID]; ok {
				info.Description = task.Description
				info.SubagentType = task.SubagentType
				info.Model = task.Model
			}
		}
		if result, ok := agentResults[agentID]; ok {
			info.Status = result.Status
			info.TotalDurationMs = result.TotalDurationMs
			info.TotalTokens = result.TotalTokens
			info.TotalToolUseCount = result.TotalToolUseCount
			if result.Usage != nil {
				info.TotalCost = turnCostFromUsage(info.Model, result.Usage)
			}
		}
		subagents = append(subagents, info)
	}
	return subagents
}

// turnCostFromUsage computes cost from a RawUsage struct and model name.
func turnCostFromUsage(model string, u *RawUsage) float64 {
	return costlog.Cost(model, u.InputTokens, u.OutputTokens, u.CacheCreationInputTokens, u.CacheReadInputTokens)
}

// LinkSubagentsToToolCalls attaches SubagentInfo pointers to matching ToolCalls.
func LinkSubagentsToToolCalls(turns []Turn, subagents []SubagentInfo) {
	// Build map of parentToolUseID → *SubagentInfo for quick lookup.
	byToolUse := make(map[string]*SubagentInfo)
	for i := range subagents {
		if subagents[i].ParentToolUseID != "" {
			byToolUse[subagents[i].ParentToolUseID] = &subagents[i]
		}
	}

	for i := range turns {
		for j := range turns[i].ToolCalls {
			tc := &turns[i].ToolCalls[j]
			if _, ok := tc.Input.(ToolInputTask); ok {
				if sa, ok := byToolUse[tc.ID]; ok {
					tc.Subagent = sa
				}
			}
		}
	}
}
