// Package format provides shared formatting functions.
package format

import "fmt"

// Bytes formats a byte count as a human-readable string (e.g. "1.5 GB").
// Uses binary units (1 KB = 1024 bytes).
func Bytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
