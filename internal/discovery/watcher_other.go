//go:build !linux

package discovery

import "log"

// Watcher is a no-op file watcher for non-Linux platforms.
//
// The Linux implementation uses inotify for real-time change notifications;
// on macOS/Windows we currently fall back to a stub that never emits events,
// so the UI works as a static viewer (refresh manually). Replacing this stub
// with kqueue (macOS) and ReadDirectoryChangesW (Windows) would restore live
// updates — see CLAUDE.md "Cross-platform watcher" for the design sketch.
type Watcher struct {
	baseDir  string
	eventsCh chan WatchEvent
	done     chan struct{}
}

// NewWatcher returns a no-op watcher whose Events channel never produces events.
func NewWatcher(baseDir string) (*Watcher, error) {
	return &Watcher{
		baseDir:  baseDir,
		eventsCh: make(chan WatchEvent),
		done:     make(chan struct{}),
	}, nil
}

// Events returns the (never-firing) event channel.
func (w *Watcher) Events() <-chan WatchEvent { return w.eventsCh }

// Start logs a one-line notice and returns. No watching occurs.
func (w *Watcher) Start() error {
	log.Printf("file watcher: live updates not supported on this platform; refresh the page to see new sessions")
	return nil
}

// Stop closes the event channel.
func (w *Watcher) Stop() {
	select {
	case <-w.done:
		return
	default:
	}
	close(w.done)
	close(w.eventsCh)
}
