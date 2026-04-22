package discovery

import "time"

// WatchEvent describes a file system change in the projects directory.
type WatchEvent struct {
	ProjectID string
	SessionID string // empty for project-level events
	Type      string // "create" or "modify"
	Time      time.Time
}
