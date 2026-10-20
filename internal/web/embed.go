// Package web embeds the user interface (HTML/CSS/JS, no external resources).
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var static embed.FS

// FS returns the interface files rooted at static/.
func FS() fs.FS {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
