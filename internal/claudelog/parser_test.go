package claudelog

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// TestParseToolInput_KnownTools
// ---------------------------------------------------------------------------

func TestParseToolInput_KnownTools(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		raw      json.RawMessage
		check    func(t *testing.T, got interface{})
	}{
		{
			name:     "Bash",
			toolName: "Bash",
			raw:      mustMarshal(map[string]string{"command": "ls -la", "description": "list files"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputBash)
				if !ok {
					t.Fatalf("type = %T, want ToolInputBash", got)
				}
				if v.Command != "ls -la" {
					t.Errorf("Command = %q, want %q", v.Command, "ls -la")
				}
				if v.Description != "list files" {
					t.Errorf("Description = %q, want %q", v.Description, "list files")
				}
			},
		},
		{
			name:     "Read",
			toolName: "Read",
			raw:      mustMarshal(map[string]string{"file_path": "/etc/hosts"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputRead)
				if !ok {
					t.Fatalf("type = %T, want ToolInputRead", got)
				}
				if v.FilePath != "/etc/hosts" {
					t.Errorf("FilePath = %q, want %q", v.FilePath, "/etc/hosts")
				}
			},
		},
		{
			name:     "Edit",
			toolName: "Edit",
			raw:      mustMarshal(map[string]string{"file_path": "f.go", "old_string": "old", "new_string": "new"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputEdit)
				if !ok {
					t.Fatalf("type = %T, want ToolInputEdit", got)
				}
				if v.FilePath != "f.go" {
					t.Errorf("FilePath = %q, want %q", v.FilePath, "f.go")
				}
				if v.OldString != "old" {
					t.Errorf("OldString = %q, want %q", v.OldString, "old")
				}
				if v.NewString != "new" {
					t.Errorf("NewString = %q, want %q", v.NewString, "new")
				}
			},
		},
		{
			name:     "Write",
			toolName: "Write",
			raw:      mustMarshal(map[string]string{"file_path": "out.txt", "content": "hello"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputWrite)
				if !ok {
					t.Fatalf("type = %T, want ToolInputWrite", got)
				}
				if v.FilePath != "out.txt" {
					t.Errorf("FilePath = %q, want %q", v.FilePath, "out.txt")
				}
				if v.Content != "hello" {
					t.Errorf("Content = %q, want %q", v.Content, "hello")
				}
			},
		},
		{
			name:     "Glob",
			toolName: "Glob",
			raw:      mustMarshal(map[string]string{"pattern": "*.go"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputGlob)
				if !ok {
					t.Fatalf("type = %T, want ToolInputGlob", got)
				}
				if v.Pattern != "*.go" {
					t.Errorf("Pattern = %q, want %q", v.Pattern, "*.go")
				}
			},
		},
		{
			name:     "Grep",
			toolName: "Grep",
			raw:      mustMarshal(map[string]string{"pattern": "TODO"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputGrep)
				if !ok {
					t.Fatalf("type = %T, want ToolInputGrep", got)
				}
				if v.Pattern != "TODO" {
					t.Errorf("Pattern = %q, want %q", v.Pattern, "TODO")
				}
			},
		},
		{
			name:     "Task",
			toolName: "Task",
			raw:      mustMarshal(map[string]string{"prompt": "do work", "description": "desc", "subagent_type": "backend-generalist"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputTask)
				if !ok {
					t.Fatalf("type = %T, want ToolInputTask", got)
				}
				if v.Prompt != "do work" {
					t.Errorf("Prompt = %q, want %q", v.Prompt, "do work")
				}
				if v.Description != "desc" {
					t.Errorf("Description = %q, want %q", v.Description, "desc")
				}
				if v.SubagentType != "backend-generalist" {
					t.Errorf("SubagentType = %q, want %q", v.SubagentType, "backend-generalist")
				}
			},
		},
		{
			name:     "WebFetch",
			toolName: "WebFetch",
			raw:      mustMarshal(map[string]string{"url": "https://example.com", "prompt": "summarize"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputWebFetch)
				if !ok {
					t.Fatalf("type = %T, want ToolInputWebFetch", got)
				}
				if v.URL != "https://example.com" {
					t.Errorf("URL = %q, want %q", v.URL, "https://example.com")
				}
				if v.Prompt != "summarize" {
					t.Errorf("Prompt = %q, want %q", v.Prompt, "summarize")
				}
			},
		},
		{
			name:     "WebSearch",
			toolName: "WebSearch",
			raw:      mustMarshal(map[string]string{"query": "go testing"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputWebSearch)
				if !ok {
					t.Fatalf("type = %T, want ToolInputWebSearch", got)
				}
				if v.Query != "go testing" {
					t.Errorf("Query = %q, want %q", v.Query, "go testing")
				}
			},
		},
		{
			name:     "TaskCreate",
			toolName: "TaskCreate",
			raw:      mustMarshal(map[string]string{"subject": "fix bug", "description": "details"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputTaskCreate)
				if !ok {
					t.Fatalf("type = %T, want ToolInputTaskCreate", got)
				}
				if v.Subject != "fix bug" {
					t.Errorf("Subject = %q, want %q", v.Subject, "fix bug")
				}
				if v.Description != "details" {
					t.Errorf("Description = %q, want %q", v.Description, "details")
				}
			},
		},
		{
			name:     "TaskUpdate",
			toolName: "TaskUpdate",
			raw:      mustMarshal(map[string]string{"taskId": "1", "status": "completed"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputTaskUpdate)
				if !ok {
					t.Fatalf("type = %T, want ToolInputTaskUpdate", got)
				}
				if v.TaskID != "1" {
					t.Errorf("TaskID = %q, want %q", v.TaskID, "1")
				}
				if v.Status != "completed" {
					t.Errorf("Status = %q, want %q", v.Status, "completed")
				}
			},
		},
		{
			name:     "TaskGet",
			toolName: "TaskGet",
			raw:      mustMarshal(map[string]string{"taskId": "1"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputTaskGet)
				if !ok {
					t.Fatalf("type = %T, want ToolInputTaskGet", got)
				}
				if v.TaskID != "1" {
					t.Errorf("TaskID = %q, want %q", v.TaskID, "1")
				}
			},
		},
		{
			name:     "TaskOutput",
			toolName: "TaskOutput",
			raw:      mustMarshal(map[string]string{"taskId": "2"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputTaskGet)
				if !ok {
					t.Fatalf("type = %T, want ToolInputTaskGet", got)
				}
				if v.TaskID != "2" {
					t.Errorf("TaskID = %q, want %q", v.TaskID, "2")
				}
			},
		},
		{
			name:     "TaskStop",
			toolName: "TaskStop",
			raw:      mustMarshal(map[string]string{"taskId": "3"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputTaskGet)
				if !ok {
					t.Fatalf("type = %T, want ToolInputTaskGet", got)
				}
				if v.TaskID != "3" {
					t.Errorf("TaskID = %q, want %q", v.TaskID, "3")
				}
			},
		},
		{
			name:     "AskUserQuestion",
			toolName: "AskUserQuestion",
			raw:      mustMarshal(map[string]interface{}{"questions": []interface{}{}}),
			check: func(t *testing.T, got interface{}) {
				_, ok := got.(ToolInputAskUser)
				if !ok {
					t.Fatalf("type = %T, want ToolInputAskUser", got)
				}
			},
		},
		{
			name:     "Skill",
			toolName: "Skill",
			raw:      mustMarshal(map[string]string{"skill": "commit"}),
			check: func(t *testing.T, got interface{}) {
				v, ok := got.(ToolInputSkill)
				if !ok {
					t.Fatalf("type = %T, want ToolInputSkill", got)
				}
				if v.Skill != "commit" {
					t.Errorf("Skill = %q, want %q", v.Skill, "commit")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseToolInput(tc.toolName, tc.raw)
			tc.check(t, got)
		})
	}
}

// ---------------------------------------------------------------------------
// TestParseToolInput_GenericTools
// ---------------------------------------------------------------------------

func TestParseToolInput_GenericTools(t *testing.T) {
	raw := mustMarshal(map[string]string{"whatever": "value"})
	tests := []struct {
		name     string
		toolName string
	}{
		{"TaskList", "TaskList"},
		{"EnterPlanMode", "EnterPlanMode"},
		{"ExitPlanMode", "ExitPlanMode"},
		{"NotebookEdit", "NotebookEdit"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseToolInput(tc.toolName, raw)
			v, ok := got.(ToolInputGeneric)
			if !ok {
				t.Fatalf("type = %T, want ToolInputGeneric", got)
			}
			// These tools return ToolInputGeneric{} with nil Raw — they don't parse the input.
			if v.Raw != nil {
				t.Errorf("Raw = %v, want nil", v.Raw)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestParseToolInput_UnknownTool
// ---------------------------------------------------------------------------

func TestParseToolInput_UnknownTool(t *testing.T) {
	raw := mustMarshal(map[string]string{"foo": "bar", "baz": "qux"})
	got := ParseToolInput("SomeNewTool", raw)
	v, ok := got.(ToolInputGeneric)
	if !ok {
		t.Fatalf("type = %T, want ToolInputGeneric", got)
	}
	if v.Raw == nil {
		t.Error("Raw = nil, want populated map for unknown tool with valid JSON object")
	}
	if v.Raw["foo"] != "bar" {
		t.Errorf("Raw[\"foo\"] = %v, want %q", v.Raw["foo"], "bar")
	}
}

// ---------------------------------------------------------------------------
// TestParseToolInput_MalformedJSON
// ---------------------------------------------------------------------------

func TestParseToolInput_MalformedJSON(t *testing.T) {
	// A valid JSON array is not an object — typed unmarshal fails AND map fallback fails.
	// ParseToolInput("Bash", [1,2,3]) → nil (genericFallback fails because array != map).
	t.Run("array_for_Bash", func(t *testing.T) {
		raw := json.RawMessage(`[1,2,3]`)
		got := ParseToolInput("Bash", raw)
		if got != nil {
			t.Errorf("got = %v (%T), want nil for array input to Bash", got, got)
		}
	})

	// A JSON object with a field Bash doesn't know about: typed unmarshal succeeds
	// (Go zero-values unknown fields), returns ToolInputBash with empty Command.
	t.Run("unknown_fields_for_Bash", func(t *testing.T) {
		raw := mustMarshal(map[string]string{"unexpected_field": "value"})
		got := ParseToolInput("Bash", raw)
		v, ok := got.(ToolInputBash)
		if !ok {
			t.Fatalf("type = %T, want ToolInputBash", got)
		}
		if v.Command != "" {
			t.Errorf("Command = %q, want empty string (zero-valued)", v.Command)
		}
	})
}

// ---------------------------------------------------------------------------
// TestParseToolInput_EmptyNil
// ---------------------------------------------------------------------------

func TestParseToolInput_EmptyNil(t *testing.T) {
	t.Run("nil_raw", func(t *testing.T) {
		got := ParseToolInput("Bash", nil)
		if got != nil {
			t.Errorf("got = %v (%T), want nil for nil raw", got, got)
		}
	})

	t.Run("empty_raw", func(t *testing.T) {
		got := ParseToolInput("Bash", json.RawMessage(""))
		if got != nil {
			t.Errorf("got = %v (%T), want nil for empty raw", got, got)
		}
	})
}

// ---------------------------------------------------------------------------
// TestParseUserContent
// ---------------------------------------------------------------------------

func TestParseUserContent_String(t *testing.T) {
	raw := mustMarshal("hello user")
	text, blocks, err := ParseUserContent(raw)
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if text != "hello user" {
		t.Errorf("text = %q, want %q", text, "hello user")
	}
	if blocks != nil {
		t.Errorf("blocks = %v, want nil", blocks)
	}
}

func TestParseUserContent_Blocks(t *testing.T) {
	input := []UserContentBlock{{Type: "tool_result", ToolUseID: "t1"}}
	raw := mustMarshal(input)
	text, blocks, err := ParseUserContent(raw)
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	if blocks[0].Type != "tool_result" {
		t.Errorf("blocks[0].Type = %q, want %q", blocks[0].Type, "tool_result")
	}
	if blocks[0].ToolUseID != "t1" {
		t.Errorf("blocks[0].ToolUseID = %q, want %q", blocks[0].ToolUseID, "t1")
	}
}

func TestParseUserContent_Empty(t *testing.T) {
	text, blocks, err := ParseUserContent(json.RawMessage(""))
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if blocks != nil {
		t.Errorf("blocks = %v, want nil", blocks)
	}
}

func TestParseUserContent_Invalid(t *testing.T) {
	_, _, err := ParseUserContent(json.RawMessage("not json"))
	if err == nil {
		t.Error("error = nil, want non-nil for invalid JSON")
	}
}

// ---------------------------------------------------------------------------
// TestParseAssistantContent
// ---------------------------------------------------------------------------

func TestParseAssistantContent_Valid(t *testing.T) {
	input := []ContentBlock{{Type: "text", Text: "hello"}}
	raw := mustMarshal(input)
	blocks, err := ParseAssistantContent(raw)
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	if blocks[0].Type != "text" {
		t.Errorf("blocks[0].Type = %q, want %q", blocks[0].Type, "text")
	}
	if blocks[0].Text != "hello" {
		t.Errorf("blocks[0].Text = %q, want %q", blocks[0].Text, "hello")
	}
}

func TestParseAssistantContent_Empty(t *testing.T) {
	blocks, err := ParseAssistantContent(json.RawMessage(""))
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if blocks != nil {
		t.Errorf("blocks = %v, want nil", blocks)
	}
}

func TestParseAssistantContent_Invalid(t *testing.T) {
	blocks, err := ParseAssistantContent(json.RawMessage("bad"))
	if err == nil {
		t.Error("error = nil, want non-nil for invalid JSON")
	}
	if blocks != nil {
		t.Errorf("blocks = %v, want nil", blocks)
	}
}

// ---------------------------------------------------------------------------
// TestExtractToolResultContent
// ---------------------------------------------------------------------------

func TestExtractToolResultContent_String(t *testing.T) {
	raw := mustMarshal("output text")
	got := ExtractToolResultContent(raw)
	if got != "output text" {
		t.Errorf("got = %q, want %q", got, "output text")
	}
}

func TestExtractToolResultContent_Blocks(t *testing.T) {
	type textBlock struct {
		Text string `json:"text"`
	}
	raw := mustMarshal([]textBlock{{Text: "part1"}, {Text: "part2"}})
	got := ExtractToolResultContent(raw)
	if got != "part1part2" {
		t.Errorf("got = %q, want %q", got, "part1part2")
	}
}

func TestExtractToolResultContent_Empty(t *testing.T) {
	got := ExtractToolResultContent(json.RawMessage(""))
	if got != "" {
		t.Errorf("got = %q, want empty", got)
	}
}

func TestExtractToolResultContent_Invalid(t *testing.T) {
	got := ExtractToolResultContent(json.RawMessage("???"))
	if got != "" {
		t.Errorf("got = %q, want empty", got)
	}
}
