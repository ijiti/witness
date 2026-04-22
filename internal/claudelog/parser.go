package claudelog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
)

const maxLineSize = 4 * 1024 * 1024 // 4MB — session lines can be large

// ParseRecords reads all records from a JSONL session file.
func ParseRecords(r io.Reader) ([]*RawRecord, error) {
	var records []*RawRecord
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec RawRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			// Skip malformed lines rather than failing the whole session.
			continue
		}
		records = append(records, &rec)
	}
	return records, scanner.Err()
}

// ParseUserContent parses the polymorphic content field of a user message.
// Returns the string text for human turns, or content blocks for tool results.
func ParseUserContent(raw json.RawMessage) (string, []UserContentBlock, error) {
	if len(raw) == 0 {
		return "", nil, nil
	}
	// Try string first (human turn).
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil, nil
	}
	// Try array of content blocks (tool results).
	var blocks []UserContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", nil, fmt.Errorf("parse user content: %w", err)
	}
	return "", blocks, nil
}

// ParseAssistantContent parses the content array from an assistant message.
func ParseAssistantContent(raw json.RawMessage) ([]ContentBlock, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("parse assistant content: %w", err)
	}
	return blocks, nil
}

// genericFallback attempts to parse raw JSON as a generic map. Returns
// ToolInputGeneric with the map on success, nil on failure. Used when
// typed unmarshal fails so templates still have data to display.
func genericFallback(raw json.RawMessage) interface{} {
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err == nil {
		return ToolInputGeneric{Raw: m}
	}
	return nil
}

// unmarshalOrFallback unmarshals raw into v. On error, logs a warning and
// returns a generic fallback. On success, returns the dereferenced value
// so callers get value types (e.g. ToolInputBash, not *ToolInputBash).
func unmarshalOrFallback[T any](name string, raw json.RawMessage) interface{} {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		log.Printf("warn: parse %s input: %v", name, err)
		return genericFallback(raw)
	}
	return v
}

// ParseToolInput parses a tool_use content block's input into a typed struct.
func ParseToolInput(name string, raw json.RawMessage) interface{} {
	if len(raw) == 0 {
		return nil
	}
	switch name {
	case "Bash":
		return unmarshalOrFallback[ToolInputBash](name, raw)
	case "Read":
		return unmarshalOrFallback[ToolInputRead](name, raw)
	case "Edit":
		return unmarshalOrFallback[ToolInputEdit](name, raw)
	case "Write":
		return unmarshalOrFallback[ToolInputWrite](name, raw)
	case "Glob":
		return unmarshalOrFallback[ToolInputGlob](name, raw)
	case "Grep":
		return unmarshalOrFallback[ToolInputGrep](name, raw)
	case "Task":
		return unmarshalOrFallback[ToolInputTask](name, raw)
	case "WebFetch":
		return unmarshalOrFallback[ToolInputWebFetch](name, raw)
	case "WebSearch":
		return unmarshalOrFallback[ToolInputWebSearch](name, raw)
	case "TaskCreate":
		return unmarshalOrFallback[ToolInputTaskCreate](name, raw)
	case "TaskUpdate":
		return unmarshalOrFallback[ToolInputTaskUpdate](name, raw)
	case "TaskGet":
		return unmarshalOrFallback[ToolInputTaskGet](name, raw)
	case "TaskList":
		return ToolInputGeneric{}
	case "TaskOutput":
		return unmarshalOrFallback[ToolInputTaskGet](name, raw)
	case "TaskStop":
		return unmarshalOrFallback[ToolInputTaskGet](name, raw)
	case "AskUserQuestion":
		return unmarshalOrFallback[ToolInputAskUser](name, raw)
	case "Skill":
		return unmarshalOrFallback[ToolInputSkill](name, raw)
	case "EnterPlanMode", "ExitPlanMode":
		return ToolInputGeneric{}
	case "NotebookEdit":
		return ToolInputGeneric{}
	default:
		// Catch-all: try to parse as generic map for unknown tools.
		return genericFallback(raw)
	}
}

// ParseProgressData parses the data field of a progress record.
func ParseProgressData(raw json.RawMessage) (*ProgressData, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var d ProgressData
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// ExtractToolResultContent extracts a human-readable summary from a user
// content block that is a tool_result. The content field is polymorphic:
// either a string or [{type:"text", text:"..."}].
func ExtractToolResultContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		parts := make([]string, 0, len(blocks))
		for _, b := range blocks {
			parts = append(parts, b.Text)
		}
		return strings.Join(parts, "")
	}
	return ""
}
