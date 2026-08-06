// Package ui embeds the built React Studio UI so the single binary serves it.
// Build the UI (npm run build in app/) and copy app/dist over ui/dist before a
// release build; a placeholder index.html keeps the embed valid in dev.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// DistFS returns the embedded UI as a filesystem rooted at the dist top level.
func DistFS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
