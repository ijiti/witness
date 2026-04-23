package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ijiti/witness/internal/discovery"
	"github.com/ijiti/witness/internal/format"
	"github.com/ijiti/witness/internal/web"
)

func main() {
	addr := format.EnvOr("WITNESS_ADDR", "127.0.0.1:7070")
	claudeDir := format.EnvOr("WITNESS_CLAUDE_DIR", defaultClaudeDir())

	disc := discovery.NewDiscoverer(claudeDir)

	// Compute fresh stats in the background so startup doesn't block.
	// GetFreshStats() returns nil until this goroutine completes; Dashboard
	// falls back to the file-based GetStats() in that window.
	go disc.ComputeFreshStats()

	// Start file watcher for live monitoring.
	if err := disc.StartWatching(); err != nil {
		log.Printf("warning: file watcher disabled: %v", err)
	}

	srv := web.NewServer(disc, addr)

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("witness starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-done
	log.Println("shutting down...")

	disc.StopWatching()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	log.Println("stopped")
}

// defaultClaudeDir returns the default location of Claude Code's project data.
//
// It mirrors Claude Code's own resolution rule:
//
//  1. If CLAUDE_CONFIG_DIR is set, use $CLAUDE_CONFIG_DIR/projects.
//  2. Otherwise use <home>/.claude/projects, where <home> is os.UserHomeDir():
//     /home/<user> on Linux, /Users/<user> on macOS, C:\Users\<user> on Windows.
func defaultClaudeDir() string {
	if cfg := os.Getenv("CLAUDE_CONFIG_DIR"); cfg != "" {
		return filepath.Join(cfg, "projects")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".claude", "projects")
	}
	return filepath.Join(home, ".claude", "projects")
}
