// Package web exposes the built Vue frontend assets to the Go side. The
// actual UI sources live in src/; the dist/ subdirectory is produced by
// `npm run build` and is gitignored except for a .gitkeep placeholder so
// `go build` succeeds on a fresh clone (with an empty UI) before the
// frontend has been built.
//
// Consumers (cmd/main.go, cmd/desktop/main.go indirectly via server/) get
// the dist/ subtree as an embed.FS via the exported FS variable.
package web

import "embed"

//go:embed all:dist
var FS embed.FS
