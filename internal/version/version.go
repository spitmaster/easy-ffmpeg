// Package version is the single source of truth for the application
// version string. Both the Web entry (cmd/main.go) and the desktop entry
// (cmd/desktop/main.go) link the same value through the shared server
// package's /api/version endpoint, so the topbar in the UI always
// reflects the current build.
//
// The default below is the marketing version of the current release.
// Build pipelines that want a richer string (e.g. "0.5.0+abcdef")
// override it via -ldflags '-X "easy-ffmpeg/internal/version.Version=..."'.
package version

// Version is the application's user-facing version string. Treated as a
// var (not const) so release builds can override it via -ldflags -X.
var Version = "0.5.1"
