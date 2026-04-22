package claudelog

import (
	"encoding/json"
	"testing"
	"time"
)

// mustMarshal marshals v to JSON, panicking on error. For test fixtures only.
func mustMarshal(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(b)
}

// makeUserRecord constructs a minimal user RawRecord with the given text content.
func makeUserRecord(uuid, sessionID, text string, ts time.Time) *RawRecord {
	return &RawRecord{
		Type:      TypeUser,
		UUID:      uuid,
		SessionID: sessionID,
		Timestamp: ts,
		Message: &RawMessage{
			Role:    "user",
			Content: mustMarshal(text),
		},
	}
}

// makeAssistantRecord constructs a minimal assistant RawRecord with text content.
func makeAssistantRecord(uuid, msgID, sessionID, text, model string, ts time.Time) *RawRecord {
	content := []ContentBlock{{Type: "text", Text: text}}
	return &RawRecord{
		Type:      TypeAssistant,
		UUID:      uuid,
		SessionID: sessionID,
		Timestamp: ts,
		Message: &RawMessage{
			Role:    "assistant",
			ID:      msgID,
			Model:   model,
			Content: mustMarshal(content),
			Usage: &RawUsage{
				InputTokens:  100,
				OutputTokens: 50,
			},
		},
	}
}

func TestBuildSessionEmpty(t *testing.T) {
	sess, err := BuildSession(nil, "proj-1")
	if err != nil {
		t.Fatalf("BuildSession(nil) error: %v", err)
	}
	if sess == nil {
		t.Fatal("BuildSession(nil) returned nil session")
	}
	if sess.ProjectID != "proj-1" {
		t.Errorf("ProjectID = %q, want %q", sess.ProjectID, "proj-1")
	}
	if len(sess.Turns) != 0 {
		t.Errorf("Turns = %d, want 0", len(sess.Turns))
	}
}

func TestBuildSessionEmptySlice(t *testing.T) {
	sess, err := BuildSession([]*RawRecord{}, "proj-x")
	if err != nil {
		t.Fatalf("BuildSession([]) error: %v", err)
	}
	if len(sess.Turns) != 0 {
		t.Errorf("Turns = %d, want 0", len(sess.Turns))
	}
}

func TestBuildSessionBasic(t *testing.T) {
	now := time.Now().UTC()
	records := []*RawRecord{
		makeUserRecord("u1", "sess-1", "hello from user", now),
		makeAssistantRecord("a1", "msg-1", "sess-1", "hello from assistant", "claude-opus-4-6", now.Add(time.Second)),
	}

	sess, err := BuildSession(records, "proj-1")
	if err != nil {
		t.Fatalf("BuildSession() error: %v", err)
	}
	if len(sess.Turns) != 1 {
		t.Fatalf("Turns = %d, want 1", len(sess.Turns))
	}

	turn := sess.Turns[0]
	if turn.UserText != "hello from user" {
		t.Errorf("UserText = %q, want %q", turn.UserText, "hello from user")
	}
	if turn.AssistantText != "hello from assistant" {
		t.Errorf("AssistantText = %q, want %q", turn.AssistantText, "hello from assistant")
	}
	if turn.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q, want %q", turn.Model, "claude-opus-4-6")
	}
	if turn.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", turn.InputTokens)
	}
	if turn.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", turn.OutputTokens)
	}
	if turn.Cost <= 0 {
		t.Errorf("Cost = %v, want > 0", turn.Cost)
	}
}

func TestBuildSessionMultipleTurns(t *testing.T) {
	now := time.Now().UTC()
	records := []*RawRecord{
		makeUserRecord("u1", "sess-1", "first question", now),
		makeAssistantRecord("a1", "msg-1", "sess-1", "first answer", "claude-opus-4-6", now.Add(time.Second)),
		makeUserRecord("u2", "sess-1", "second question", now.Add(2*time.Second)),
		makeAssistantRecord("a2", "msg-2", "sess-1", "second answer", "claude-opus-4-6", now.Add(3*time.Second)),
	}

	sess, err := BuildSession(records, "proj-1")
	if err != nil {
		t.Fatalf("BuildSession() error: %v", err)
	}
	if len(sess.Turns) != 2 {
		t.Fatalf("Turns = %d, want 2", len(sess.Turns))
	}
	if sess.Turns[0].UserText != "first question" {
		t.Errorf("Turn[0].UserText = %q, want %q", sess.Turns[0].UserText, "first question")
	}
	if sess.Turns[1].UserText != "second question" {
		t.Errorf("Turn[1].UserText = %q, want %q", sess.Turns[1].UserText, "second question")
	}
}

func TestMergeStreamingRecords(t *testing.T) {
	now := time.Now().UTC()
	sharedMsgID := "msg-streaming-1"

	// Two assistant records sharing the same message ID — simulating streaming.
	r1 := makeAssistantRecord("a1", sharedMsgID, "sess-1", "part one", "claude-opus-4-6", now)
	r1.Message.Usage = &RawUsage{InputTokens: 50, OutputTokens: 20}

	r2 := makeAssistantRecord("a2", sharedMsgID, "sess-1", "part two", "claude-opus-4-6", now.Add(100*time.Millisecond))
	r2.Message.Usage = &RawUsage{InputTokens: 200, OutputTokens: 80} // higher — should win

	records := []*RawRecord{r1, r2}
	merged := mergeStreamingRecords(records)

	if len(merged) != 1 {
		t.Fatalf("mergeStreamingRecords() = %d records, want 1 (merged)", len(merged))
	}

	// Content should include both text blocks.
	blocks, err := ParseAssistantContent(merged[0].Message.Content)
	if err != nil {
		t.Fatalf("ParseAssistantContent error: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("merged content blocks = %d, want 2", len(blocks))
	}
	if blocks[0].Text != "part one" {
		t.Errorf("blocks[0].Text = %q, want %q", blocks[0].Text, "part one")
	}
	if blocks[1].Text != "part two" {
		t.Errorf("blocks[1].Text = %q, want %q", blocks[1].Text, "part two")
	}

	// Best usage (highest total) should be from r2.
	if merged[0].Message.Usage == nil {
		t.Fatal("merged record has nil Usage")
	}
	if merged[0].Message.Usage.InputTokens != 200 {
		t.Errorf("merged Usage.InputTokens = %d, want 200 (from best record)", merged[0].Message.Usage.InputTokens)
	}

	// UUID should be from first record.
	if merged[0].UUID != "a1" {
		t.Errorf("merged UUID = %q, want %q (from first record)", merged[0].UUID, "a1")
	}
}

func TestMergeStreamingRecordsDifferentIDs(t *testing.T) {
	// Records with different message IDs should NOT be merged.
	now := time.Now().UTC()
	r1 := makeAssistantRecord("a1", "msg-1", "sess-1", "first", "claude-opus-4-6", now)
	r2 := makeAssistantRecord("a2", "msg-2", "sess-1", "second", "claude-opus-4-6", now.Add(time.Second))

	merged := mergeStreamingRecords([]*RawRecord{r1, r2})
	if len(merged) != 2 {
		t.Errorf("mergeStreamingRecords() = %d records, want 2 (no merge)", len(merged))
	}
}

func TestCompactionMarker(t *testing.T) {
	now := time.Now().UTC()

	// Sequence: user → assistant → compact_boundary system record → user (should have CompactionBefore=true)
	compactBoundaryUUID := "compact-uuid-1"
	records := []*RawRecord{
		makeUserRecord("u1", "sess-1", "before compaction", now),
		makeAssistantRecord("a1", "msg-1", "sess-1", "response before", "claude-opus-4-6", now.Add(time.Second)),
		{
			Type:      TypeSystem,
			UUID:      compactBoundaryUUID,
			SessionID: "sess-1",
			Timestamp: now.Add(2 * time.Second),
			Subtype:   SubtypeCompactBoundary,
		},
		makeUserRecord("u2", "sess-1", "after compaction", now.Add(3*time.Second)),
		makeAssistantRecord("a2", "msg-2", "sess-1", "response after", "claude-opus-4-6", now.Add(4*time.Second)),
	}

	sess, err := BuildSession(records, "proj-1")
	if err != nil {
		t.Fatalf("BuildSession() error: %v", err)
	}
	if len(sess.Turns) != 2 {
		t.Fatalf("Turns = %d, want 2", len(sess.Turns))
	}

	// First turn: before compaction — no CompactionBefore marker.
	if sess.Turns[0].CompactionBefore {
		t.Error("Turns[0].CompactionBefore = true, want false (before compaction boundary)")
	}

	// Second turn: immediately after compaction boundary — should have marker.
	if !sess.Turns[1].CompactionBefore {
		t.Error("Turns[1].CompactionBefore = false, want true (first user record after compact_boundary)")
	}

	// Compactions counter should be incremented.
	if sess.Compactions != 1 {
		t.Errorf("Compactions = %d, want 1", sess.Compactions)
	}
}

func TestCompactionMarkerOnlyFirst(t *testing.T) {
	now := time.Now().UTC()

	// After a compaction boundary, only the first non-meta user record gets the marker.
	records := []*RawRecord{
		{
			Type:      TypeSystem,
			UUID:      "compact-1",
			SessionID: "sess-1",
			Timestamp: now,
			Subtype:   SubtypeCompactBoundary,
		},
		makeUserRecord("u1", "sess-1", "first after compaction", now.Add(time.Second)),
		makeAssistantRecord("a1", "msg-1", "sess-1", "answer1", "claude-opus-4-6", now.Add(2*time.Second)),
		makeUserRecord("u2", "sess-1", "second after compaction", now.Add(3*time.Second)),
		makeAssistantRecord("a2", "msg-2", "sess-1", "answer2", "claude-opus-4-6", now.Add(4*time.Second)),
	}

	sess, err := BuildSession(records, "proj-1")
	if err != nil {
		t.Fatalf("BuildSession() error: %v", err)
	}
	if len(sess.Turns) != 2 {
		t.Fatalf("Turns = %d, want 2", len(sess.Turns))
	}

	if !sess.Turns[0].CompactionBefore {
		t.Error("Turns[0].CompactionBefore = false, want true")
	}
	if sess.Turns[1].CompactionBefore {
		t.Error("Turns[1].CompactionBefore = true, want false (only first turn after compaction gets marker)")
	}
}
