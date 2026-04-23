// Package claudelog provides types and parsing for Claude Code session JSONL files.
package claudelog

import (
	"encoding/json"
	"fmt"
	"time"
)

// Record types found in session JSONL files.
const (
	TypeUser                = "user"
	TypeAssistant           = "assistant"
	TypeSystem              = "system"
	TypeProgress            = "progress"
	TypeAgentSetting = "agent-setting"
	TypeCustomTitle  = "custom-title"
)

// System subtypes.
const (
	SubtypeTurnDuration    = "turn_duration"
	SubtypeCompactBoundary = "compact_boundary"
)

// RawRecord is the outer envelope for every line in a session JSONL file.
type RawRecord struct {
	Type      string    `json:"type"`
	UUID      string    `json:"uuid"`
	ParentUUID *string  `json:"parentUuid"`
	SessionID string    `json:"sessionId"`
	Timestamp time.Time `json:"timestamp"`

	IsSidechain bool   `json:"isSidechain"`
	UserType    string `json:"userType"`
	CWD         string `json:"cwd"`
	Version     string `json:"version"`
	GitBranch   string `json:"gitBranch"`
	Slug        string `json:"slug"`
	AgentID     string `json:"agentId,omitempty"`

	// Present on user and assistant records.
	Message *RawMessage `json:"message,omitempty"`

	// System record fields.
	Subtype    string `json:"subtype,omitempty"`
	Level      string `json:"level,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`

	// Content field — used differently by system records (string) and
	// compact_boundary (string). Kept as RawMessage for flexibility.
	Content json.RawMessage `json:"content,omitempty"`

	// Compact boundary metadata.
	LogicalParentUUID *string          `json:"logicalParentUuid,omitempty"`
	CompactMetadata   json.RawMessage  `json:"compactMetadata,omitempty"`

	// User record metadata.
	IsMeta           bool   `json:"isMeta,omitempty"`
	IsCompactSummary bool   `json:"isCompactSummary,omitempty"`
	PermissionMode   string `json:"permissionMode,omitempty"`

	// Tool result enrichment on user records.
	ToolUseResult           json.RawMessage `json:"toolUseResult,omitempty"`
	SourceToolAssistantUUID string          `json:"sourceToolAssistantUUID,omitempty"`

	// Assistant record fields.
	RequestID       string `json:"requestId,omitempty"`
	IsAPIErrorMsg   bool   `json:"isApiErrorMessage,omitempty"`

	// Progress record fields.
	Data            json.RawMessage `json:"data,omitempty"`
	ToolUseID       string          `json:"toolUseID,omitempty"`
	ParentToolUseID string          `json:"parentToolUseID,omitempty"`

	// Queue operation fields.
	Operation string `json:"operation,omitempty"`

	// Agent setting fields.
	AgentSetting string `json:"agentSetting,omitempty"`

	// Custom title fields.
	CustomTitle string `json:"customTitle,omitempty"`

	// File history snapshot fields.
	MessageID        string          `json:"messageId,omitempty"`
	IsSnapshotUpdate bool            `json:"isSnapshotUpdate,omitempty"`
	Snapshot         json.RawMessage `json:"snapshot,omitempty"`
}

// RawMessage is the "message" field on user and assistant records.
type RawMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ID         string          `json:"id,omitempty"`
	Model      string          `json:"model,omitempty"`
	Type       string          `json:"type,omitempty"`
	StopReason *string         `json:"stop_reason"`
	Usage      *RawUsage       `json:"usage,omitempty"`
}

// RawUsage is token usage reported on assistant messages.
type RawUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// ContentBlock is a parsed element from an assistant message's content array.
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
}

// UserContentBlock is a parsed element from a user message's content array.
type UserContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// ProgressData is the parsed "data" field from progress records.
type ProgressData struct {
	Type            string          `json:"type"`
	HookEvent       string          `json:"hookEvent,omitempty"`
	HookName        string          `json:"hookName,omitempty"`
	Command         string          `json:"command,omitempty"`
	Output          string          `json:"output,omitempty"`
	FullOutput      string          `json:"fullOutput,omitempty"`
	Prompt          string          `json:"prompt,omitempty"`
	AgentID         string          `json:"agentId,omitempty"`
	TaskDescription string          `json:"taskDescription,omitempty"`
	Message         json.RawMessage `json:"message,omitempty"`
}

// --- Tool input types (parsed from tool_use content blocks) ---

// ToolInputBash is the input for Bash tool calls.
type ToolInputBash struct {
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
	Timeout     int    `json:"timeout,omitempty"`
}

// ToolInputRead is the input for Read tool calls.
type ToolInputRead struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// ToolInputEdit is the input for Edit tool calls.
type ToolInputEdit struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// ToolInputWrite is the input for Write tool calls.
type ToolInputWrite struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// ToolInputGlob is the input for Glob tool calls.
type ToolInputGlob struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

// ToolInputGrep is the input for Grep tool calls.
type ToolInputGrep struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	Glob       string `json:"glob,omitempty"`
	OutputMode string `json:"output_mode,omitempty"`
}

// ToolInputTask is the input for Task tool calls.
type ToolInputTask struct {
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
	SubagentType string `json:"subagent_type,omitempty"`
	Model        string `json:"model,omitempty"`
}

// ToolInputWebFetch is the input for WebFetch tool calls.
type ToolInputWebFetch struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt"`
}

// ToolInputWebSearch is the input for WebSearch tool calls.
type ToolInputWebSearch struct {
	Query string `json:"query"`
}

// ToolInputTaskCreate is the input for TaskCreate tool calls.
type ToolInputTaskCreate struct {
	Subject     string `json:"subject"`
	Description string `json:"description"`
	ActiveForm  string `json:"activeForm,omitempty"`
}

// ToolInputTaskUpdate is the input for TaskUpdate tool calls.
type ToolInputTaskUpdate struct {
	TaskID      string   `json:"taskId"`
	Status      string   `json:"status,omitempty"`
	Subject     string   `json:"subject,omitempty"`
	Description string   `json:"description,omitempty"`
	AddBlocks   []string `json:"addBlocks,omitempty"`
	AddBlockedBy []string `json:"addBlockedBy,omitempty"`
}

// ToolInputTaskGet is the input for TaskGet tool calls.
type ToolInputTaskGet struct {
	TaskID string `json:"taskId"`
}

// ToolInputAskUser is the input for AskUserQuestion tool calls.
type ToolInputAskUser struct {
	Questions []struct {
		Question string `json:"question"`
		Options  []struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"options"`
	} `json:"questions"`
}

// ToolInputSkill is the input for Skill tool calls.
type ToolInputSkill struct {
	Skill string `json:"skill"`
	Args  string `json:"args,omitempty"`
}

// ToolInputGeneric is a fallback for unrecognized tool inputs.
type ToolInputGeneric struct {
	Raw map[string]interface{}
}

// --- Domain types (built from parsed records) ---

// Session represents a fully parsed Claude Code session.
type Session struct {
	ID        string
	ProjectID string
	Slug      string
	Title     string
	CWD       string
	GitBranch string
	Version   string
	Model     string
	StartTime time.Time
	EndTime   time.Time
	Turns     []Turn

	TotalInputTokens  int
	TotalOutputTokens int
	TotalCacheCreate  int
	TotalCacheRead    int

	TotalCost    float64
	SubagentCost float64
	MaxDuration  time.Duration

	HasSubagents bool
	Subagents    []SubagentInfo
	AgentSetting string
	AgentPersona *AgentPersona
	PlanPath     string
	Compactions  int
}

// SubagentInfo describes a subagent spawned by a Task tool call.
type SubagentInfo struct {
	AgentID           string
	ParentToolUseID   string
	Description       string
	SubagentType      string
	Model             string
	Status            string
	TotalDurationMs   int64
	TotalTokens       int
	TotalToolUseCount int
	TotalCost         float64
	FilePath          string
	ViewURL           string
}

// Turn represents one user→assistant exchange.
type Turn struct {
	Index     int
	Timestamp time.Time
	Duration  time.Duration

	UserText         string
	IsCompactSummary bool
	CompactionBefore bool // a context compaction occurred just before this turn

	AssistantText string
	ThinkingText  string
	// ThinkingBlocks counts thinking content blocks in the assistant message,
	// regardless of whether their text is captured. Claude Code may persist
	// signature-only thinking blocks (empty text + cryptographic signature)
	// when the user has not opted into showThinkingSummaries; in that case
	// ThinkingText is empty but ThinkingBlocks > 0 — surface the indicator
	// instead of silently hiding the fact that the agent thought.
	ThinkingBlocks int
	Model          string

	ToolCalls []ToolCall

	InputTokens  int
	OutputTokens int
	CacheCreate  int
	CacheRead    int
	Cost         float64
}

// ToolCall represents one tool invocation and its result.
type ToolCall struct {
	ID       string
	Name     string
	Input    interface{}
	Result   *ToolResult
	Subagent *SubagentInfo
}

// ToolCallSummary returns a short human-readable summary of the tool call.
func (tc ToolCall) Summary() string {
	switch v := tc.Input.(type) {
	case ToolInputBash:
		if v.Description != "" {
			return v.Description
		}
		if len(v.Command) > 80 {
			return v.Command[:77] + "..."
		}
		return v.Command
	case ToolInputRead:
		return v.FilePath
	case ToolInputEdit:
		return v.FilePath
	case ToolInputWrite:
		return v.FilePath
	case ToolInputGlob:
		return v.Pattern
	case ToolInputGrep:
		s := v.Pattern
		if v.Path != "" {
			s += " in " + v.Path
		}
		return s
	case ToolInputTask:
		return v.Description
	case ToolInputWebFetch:
		return v.URL
	case ToolInputWebSearch:
		return v.Query
	case ToolInputTaskCreate:
		return v.Subject
	case ToolInputTaskUpdate:
		s := "#" + v.TaskID
		if v.Status != "" {
			s += " → " + v.Status
		}
		if v.Subject != "" {
			s += " " + v.Subject
		}
		return s
	case ToolInputTaskGet:
		return "#" + v.TaskID
	case ToolInputAskUser:
		if len(v.Questions) > 0 {
			return v.Questions[0].Question
		}
		return "asking user"
	case ToolInputSkill:
		s := "/" + v.Skill
		if v.Args != "" {
			s += " " + v.Args
		}
		return s
	case ToolInputGeneric:
		return ""
	default:
		return ""
	}
}

// Detail returns a structured multi-line description of the tool call input.
func (tc ToolCall) Detail() string {
	switch v := tc.Input.(type) {
	case ToolInputBash:
		return v.Command
	case ToolInputRead:
		s := v.FilePath
		if v.Offset > 0 || v.Limit > 0 {
			s += fmt.Sprintf(" (offset:%d limit:%d)", v.Offset, v.Limit)
		}
		return s
	case ToolInputEdit:
		return fmt.Sprintf("file: %s\n--- old\n%s\n+++ new\n%s", v.FilePath, v.OldString, v.NewString)
	case ToolInputWrite:
		if len(v.Content) > 500 {
			return fmt.Sprintf("file: %s\n%s...", v.FilePath, v.Content[:500])
		}
		return fmt.Sprintf("file: %s\n%s", v.FilePath, v.Content)
	case ToolInputGlob:
		s := v.Pattern
		if v.Path != "" {
			s += " in " + v.Path
		}
		return s
	case ToolInputGrep:
		s := "pattern: " + v.Pattern
		if v.Path != "" {
			s += "\npath: " + v.Path
		}
		if v.Glob != "" {
			s += "\nglob: " + v.Glob
		}
		return s
	case ToolInputTask:
		s := v.Description
		if v.SubagentType != "" {
			s += "\nagent: " + v.SubagentType
		}
		if v.Model != "" {
			s += "\nmodel: " + v.Model
		}
		if v.Prompt != "" {
			prompt := v.Prompt
			if len(prompt) > 500 {
				prompt = prompt[:500] + "..."
			}
			s += "\n\n" + prompt
		}
		return s
	case ToolInputWebFetch:
		return fmt.Sprintf("url: %s\nprompt: %s", v.URL, v.Prompt)
	case ToolInputWebSearch:
		return v.Query
	case ToolInputTaskCreate:
		s := v.Subject
		if v.Description != "" {
			s += "\n" + v.Description
		}
		return s
	case ToolInputTaskUpdate:
		s := "task #" + v.TaskID
		if v.Status != "" {
			s += " → " + v.Status
		}
		if v.Subject != "" {
			s += "\nsubject: " + v.Subject
		}
		if len(v.AddBlockedBy) > 0 {
			s += fmt.Sprintf("\nblockedBy: %v", v.AddBlockedBy)
		}
		if len(v.AddBlocks) > 0 {
			s += fmt.Sprintf("\nblocks: %v", v.AddBlocks)
		}
		return s
	case ToolInputTaskGet:
		return "task #" + v.TaskID
	case ToolInputAskUser:
		if len(v.Questions) == 0 {
			return ""
		}
		s := v.Questions[0].Question
		for _, opt := range v.Questions[0].Options {
			s += "\n  - " + opt.Label
			if opt.Description != "" {
				s += ": " + opt.Description
			}
		}
		return s
	case ToolInputSkill:
		return "/" + v.Skill
	case ToolInputGeneric:
		return ""
	default:
		return ""
	}
}

// ToolResult holds the outcome of a tool call.
type ToolResult struct {
	ToolUseID string
	IsError   bool
	Content   string
}
