package discovery

import (
	"sync"
	"testing"
	"time"
)

func TestActivityTracker_MarkAndQuery(t *testing.T) {
	tr := NewActivityTracker()
	base := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return base }

	tr.Mark("proj-a", "sess-1")

	if !tr.IsProjectActive("proj-a") {
		t.Fatal("proj-a should be active immediately after Mark")
	}
	if !tr.IsSessionActive("proj-a", "sess-1") {
		t.Fatal("sess-1 should be active immediately after Mark")
	}
	if tr.IsProjectActive("proj-b") {
		t.Fatal("proj-b should not be active")
	}
	if tr.IsSessionActive("proj-a", "sess-other") {
		t.Fatal("sess-other should not be active")
	}
}

func TestActivityTracker_ExpiresOutsideWindow(t *testing.T) {
	tr := NewActivityTracker()
	base := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return base }

	tr.Mark("proj-a", "sess-1")

	// Jump past the window.
	tr.now = func() time.Time { return base.Add(ActivityWindow + time.Second) }
	if tr.IsProjectActive("proj-a") {
		t.Fatal("proj-a should no longer be active after window expires")
	}
	if tr.IsSessionActive("proj-a", "sess-1") {
		t.Fatal("sess-1 should no longer be active after window expires")
	}
}

func TestActivityTracker_StaysActiveAtBoundary(t *testing.T) {
	tr := NewActivityTracker()
	base := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return base }
	tr.Mark("proj-a", "sess-1")

	tr.now = func() time.Time { return base.Add(ActivityWindow) }
	if !tr.IsProjectActive("proj-a") {
		t.Fatal("proj-a should remain active exactly at the window boundary")
	}
}

func TestActivityTracker_ProjectLevelEvent(t *testing.T) {
	tr := NewActivityTracker()
	base := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return base }

	tr.Mark("proj-a", "") // project-only event (new session file appearing)
	if !tr.IsProjectActive("proj-a") {
		t.Fatal("project-level Mark should make project active")
	}
	// No specific session should be flagged active.
	if tr.IsSessionActive("proj-a", "anything") {
		t.Fatal("project-level Mark should not flag any session")
	}
}

func TestActivityTracker_EmptyProjectIDIgnored(t *testing.T) {
	tr := NewActivityTracker()
	tr.Mark("", "sess-orphan")
	if len(tr.ActiveProjects()) != 0 || len(tr.ActiveSessions()) != 0 {
		t.Fatal("empty projectID should be ignored")
	}
}

func TestActivityTracker_ActiveProjectsAndSessions(t *testing.T) {
	tr := NewActivityTracker()
	base := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return base }

	tr.Mark("proj-a", "s1")
	tr.Mark("proj-a", "s2")
	tr.Mark("proj-b", "s3")

	// Expire proj-a/s1 but keep everything else fresh.
	tr.now = func() time.Time { return base.Add(10 * time.Second) }
	tr.Mark("proj-a", "s2")
	tr.Mark("proj-b", "s3")

	// Skip past the initial window so only the re-marked keys remain.
	tr.now = func() time.Time { return base.Add(ActivityWindow + 5*time.Second) }

	projects := tr.ActiveProjects()
	if !projects["proj-a"] || !projects["proj-b"] {
		t.Fatalf("both projects should be active; got %v", projects)
	}

	sessions := tr.ActiveSessions()
	if sessions["proj-a/s1"] {
		t.Fatalf("s1 should have expired; got active set %v", sessions)
	}
	if !sessions["proj-a/s2"] || !sessions["proj-b/s3"] {
		t.Fatalf("s2 and s3 should be active; got %v", sessions)
	}
}

func TestActivityTracker_ConcurrentMarks(t *testing.T) {
	tr := NewActivityTracker()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tr.Mark("proj", "sess")
			_ = tr.IsProjectActive("proj")
			_ = tr.ActiveProjects()
			_ = tr.ActiveSessions()
		}(i)
	}
	wg.Wait()

	if !tr.IsProjectActive("proj") {
		t.Fatal("proj should be active after concurrent marks")
	}
}
