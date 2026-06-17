package web

import "io/fs"

// NewRendererFS exposes the filesystem-injectable constructor so tests can drive
// the parse-error path with a filesystem that lacks the templates.
func NewRendererFS(fsys fs.FS) (*Renderer, error) {
	return newRenderer(fsys)
}
