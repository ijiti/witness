// Package build holds the build-time identity variable for witness.
//
// BuildID is embedded via ldflags at release time:
//
//	go build -ldflags="-X 'github.com/ijiti/witness/internal/build.BuildID=v0.2.0'"
//
// When no ldflags are passed (e.g. during development), BuildID defaults to
// "dev". Production binaries use the binary's own mtime as a fallback so that
// every `go build` produces a distinct ETag namespace even without ldflags.
package build

// BuildID is set at link time by the release workflow. Its default value "dev"
// is intentionally stable so that development builds produce consistent ETags
// across process restarts (avoiding spurious cache-busts during local work).
var BuildID = "dev"
