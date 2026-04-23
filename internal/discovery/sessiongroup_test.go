package discovery

import (
	"testing"
	"time"
)

func TestGroupSessionsByDate(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, 4, 23, 14, 30, 0, 0, loc)

	sess := func(id string, mt time.Time) SessionEntry {
		return SessionEntry{ID: id, ModTime: mt}
	}

	sessions := []SessionEntry{
		sess("today-afternoon", now.Add(-2*time.Hour)),                // Today
		sess("today-morning", time.Date(2026, 4, 23, 3, 0, 0, 0, loc)), // Today
		sess("yesterday-late", time.Date(2026, 4, 22, 23, 55, 0, 0, loc)),
		sess("yesterday-early", time.Date(2026, 4, 22, 1, 0, 0, 0, loc)),
		sess("this-week-3days", time.Date(2026, 4, 20, 12, 0, 0, 0, loc)),
		sess("this-week-6days", time.Date(2026, 4, 17, 12, 0, 0, 0, loc)),
		sess("older-1", time.Date(2026, 4, 15, 9, 0, 0, 0, loc)),
		sess("older-2", time.Date(2025, 12, 1, 9, 0, 0, 0, loc)),
	}

	got := GroupSessionsByDate(sessions, now)

	if len(got) != 4 {
		t.Fatalf("expected 4 groups, got %d: %+v", len(got), got)
	}
	labels := []string{"Today", "Yesterday", "This week", "Older"}
	for i, g := range got {
		if g.Label != labels[i] {
			t.Errorf("group %d label = %q, want %q", i, g.Label, labels[i])
		}
	}

	want := map[string][]string{
		"Today":     {"today-afternoon", "today-morning"},
		"Yesterday": {"yesterday-late", "yesterday-early"},
		"This week": {"this-week-3days", "this-week-6days"},
		"Older":     {"older-1", "older-2"},
	}
	for _, g := range got {
		ids := make([]string, len(g.Sessions))
		for i, s := range g.Sessions {
			ids[i] = s.ID
		}
		expected := want[g.Label]
		if len(ids) != len(expected) {
			t.Errorf("%s: got %v, want %v", g.Label, ids, expected)
			continue
		}
		for i, id := range ids {
			if id != expected[i] {
				t.Errorf("%s[%d] = %q, want %q", g.Label, i, id, expected[i])
			}
		}
	}
}

func TestGroupSessionsByDate_EmptyBucketsOmitted(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, 4, 23, 14, 30, 0, 0, loc)

	// Only older sessions — expect exactly one group, "Older".
	sessions := []SessionEntry{
		{ID: "a", ModTime: time.Date(2026, 1, 1, 0, 0, 0, 0, loc)},
		{ID: "b", ModTime: time.Date(2025, 6, 1, 0, 0, 0, 0, loc)},
	}
	got := GroupSessionsByDate(sessions, now)
	if len(got) != 1 || got[0].Label != "Older" {
		t.Fatalf("expected single Older group, got %+v", got)
	}
	if len(got[0].Sessions) != 2 {
		t.Fatalf("expected 2 sessions in Older, got %d", len(got[0].Sessions))
	}
}

func TestGroupSessionsByDate_Empty(t *testing.T) {
	got := GroupSessionsByDate(nil, time.Now())
	if got != nil {
		t.Fatalf("expected nil for empty input, got %+v", got)
	}
}

func TestGroupSessionsByDate_BoundaryYesterdayMidnight(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, 4, 23, 14, 30, 0, 0, loc)
	// Exactly midnight of yesterday — should be in Yesterday, not This week.
	s := SessionEntry{ID: "boundary", ModTime: time.Date(2026, 4, 22, 0, 0, 0, 0, loc)}
	got := GroupSessionsByDate([]SessionEntry{s}, now)
	if len(got) != 1 || got[0].Label != "Yesterday" {
		t.Fatalf("expected Yesterday bucket, got %+v", got)
	}
}
