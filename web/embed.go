// Package web embeds the built UI (web/dist) into the Go binary so the
// server can serve it at "/". Build it first with `npm run build` inside
// web/; without a build, only dist/.gitkeep is embedded and the server
// answers with a short notice instead of the UI.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built UI rooted at dist/.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // invariant: "dist" is embedded above, so Sub cannot fail
	}
	return sub
}

// Built reports whether the UI was built before compiling the binary.
func Built() bool {
	_, err := fs.Stat(FS(), "index.html")
	return err == nil
}
