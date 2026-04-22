//go:build linux

package discovery

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Watcher monitors ~/.claude/projects/ for JSONL file changes using inotify.
type Watcher struct {
	fd       int
	baseDir  string
	mu       sync.RWMutex
	watches  map[int]string // watch descriptor → directory path
	paths    map[string]int // directory path → watch descriptor
	eventsCh chan WatchEvent
	done     chan struct{}
}

// NewWatcher creates a new inotify-based file watcher.
func NewWatcher(baseDir string) (*Watcher, error) {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}
	return &Watcher{
		fd:       fd,
		baseDir:  baseDir,
		watches:  make(map[int]string),
		paths:    make(map[string]int),
		eventsCh: make(chan WatchEvent, 64),
		done:     make(chan struct{}),
	}, nil
}

// Events returns the channel of watch events.
func (w *Watcher) Events() <-chan WatchEvent {
	return w.eventsCh
}

// Start begins watching the base directory and all project subdirectories.
func (w *Watcher) Start() error {
	// Watch the base directory for new project directories.
	if err := w.addWatch(w.baseDir); err != nil {
		return err
	}

	// Watch each existing project subdirectory.
	entries, err := os.ReadDir(w.baseDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(w.baseDir, e.Name())
		if err := w.addWatch(dir); err != nil {
			log.Printf("watcher: addWatch %s: %v", dir, err)
		}
	}

	go w.readLoop()
	return nil
}

// Stop shuts down the watcher.
func (w *Watcher) Stop() {
	close(w.done)
	syscall.Close(w.fd)
}

const watchMask = syscall.IN_MODIFY | syscall.IN_CREATE | syscall.IN_MOVED_TO

func (w *Watcher) addWatch(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.paths[path]; exists {
		return nil // already watching
	}

	wd, err := syscall.InotifyAddWatch(w.fd, path, watchMask)
	if err != nil {
		return err
	}
	w.watches[wd] = path
	w.paths[path] = wd
	return nil
}

// readLoop polls the inotify fd and parses events.
func (w *Watcher) readLoop() {
	defer close(w.eventsCh)
	buf := make([]byte, 4096)

	for {
		select {
		case <-w.done:
			return
		default:
		}

		n, err := syscall.Read(w.fd, buf)
		if err != nil {
			if err == syscall.EAGAIN {
				// Non-blocking: no events ready, sleep briefly.
				time.Sleep(100 * time.Millisecond)
				continue
			}
			// fd closed or real error
			select {
			case <-w.done:
				return
			default:
				log.Printf("watcher: read error: %v", err)
				time.Sleep(time.Second)
				continue
			}
		}
		if n < syscall.SizeofInotifyEvent {
			continue
		}

		w.parseEvents(buf[:n])
	}
}

func (w *Watcher) parseEvents(buf []byte) {
	offset := 0
	for offset+syscall.SizeofInotifyEvent <= len(buf) {
		raw := (*syscall.InotifyEvent)(unsafe.Pointer(&buf[offset]))
		offset += syscall.SizeofInotifyEvent

		var name string
		if raw.Len > 0 {
			if offset+int(raw.Len) > len(buf) {
				break
			}
			nameBytes := buf[offset : offset+int(raw.Len)]
			name = string(bytes.TrimRight(nameBytes, "\x00"))
			// Advance by padded length (4-byte aligned).
			offset += int((raw.Len + 3) / 4 * 4)
		}

		w.mu.RLock()
		dir := w.watches[int(raw.Wd)]
		w.mu.RUnlock()

		if dir == "" {
			continue
		}

		// Determine event type.
		var evType string
		if raw.Mask&syscall.IN_CREATE != 0 || raw.Mask&syscall.IN_MOVED_TO != 0 {
			evType = "create"
		} else if raw.Mask&syscall.IN_MODIFY != 0 {
			evType = "modify"
		}

		if evType == "" {
			continue
		}

		// New directory created in base → add watch for it.
		if raw.Mask&syscall.IN_ISDIR != 0 && evType == "create" {
			newDir := filepath.Join(dir, name)
			w.addWatch(newDir)
			continue
		}

		// Only care about .jsonl files.
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}

		// Determine project and session IDs from directory structure.
		projectID, sessionID := w.resolveIDs(dir, name)
		if projectID == "" {
			continue
		}

		ev := WatchEvent{
			ProjectID: projectID,
			SessionID: sessionID,
			Type:      evType,
			Time:      time.Now(),
		}

		select {
		case w.eventsCh <- ev:
		default:
			// Channel full — drop event (subscriber will catch up).
		}
	}
}

// resolveIDs maps a directory and filename to project/session IDs.
func (w *Watcher) resolveIDs(dir, name string) (string, string) {
	sessionID := strings.TrimSuffix(name, ".jsonl")

	// If dir is the base directory, this shouldn't happen for .jsonl files.
	if dir == w.baseDir {
		return "", ""
	}

	projectID := filepath.Base(dir)
	return projectID, sessionID
}
