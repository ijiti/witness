package discovery

import "time"

// SessionGroup is a bucket of SessionEntries sharing a coarse date label
// ("Today", "Yesterday", "This week", "Older"). Buckets are produced in
// descending recency so templates can render them straight through.
type SessionGroup struct {
	Label    string
	Sessions []SessionEntry
}

// GroupSessionsByDate partitions a slice of SessionEntries (assumed sorted
// most-recent-first) into the four coarse buckets the sidebar uses. "Today"
// and "Yesterday" are calendar-day buckets in the local timezone — so a
// session that ran at 23:55 yesterday stays in "Yesterday" even 61 minutes
// later. "This week" covers the previous six calendar days; everything older
// falls into "Older". Empty buckets are omitted so a list with only older
// sessions never renders four empty headers.
func GroupSessionsByDate(sessions []SessionEntry, now time.Time) []SessionGroup {
	if len(sessions) == 0 {
		return nil
	}
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	yesterday := today.AddDate(0, 0, -1)
	weekStart := today.AddDate(0, 0, -7)

	groups := []SessionGroup{
		{Label: "Today"},
		{Label: "Yesterday"},
		{Label: "This week"},
		{Label: "Older"},
	}

	for _, s := range sessions {
		mt := s.ModTime.In(loc)
		switch {
		case !mt.Before(today):
			groups[0].Sessions = append(groups[0].Sessions, s)
		case !mt.Before(yesterday):
			groups[1].Sessions = append(groups[1].Sessions, s)
		case !mt.Before(weekStart):
			groups[2].Sessions = append(groups[2].Sessions, s)
		default:
			groups[3].Sessions = append(groups[3].Sessions, s)
		}
	}

	out := make([]SessionGroup, 0, len(groups))
	for _, g := range groups {
		if len(g.Sessions) > 0 {
			out = append(out, g)
		}
	}
	return out
}
