// Package web holds the server-rendered UI (ADR-0002, PLAN §4): the design-system
// primitives — the racing-flag badge, the layout partials, the token/font assets
// — plus the throwaway styleguide. Feature screens are added by the phases that
// own their backend; this package provides the shared rendering plumbing they
// build on.
package web

import (
	"embed"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"net/http"
)

//go:embed templates components
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// ErrUnknownStatus is returned by RenderBadge for a non-flag status token.
var ErrUnknownStatus = errors.New("web: unknown status token")

// Renderer holds every parsed page and component in one template set. Pages
// render themselves by composing the shared "head"/"foot" layout partials, so
// there are no per-page "content" collisions and no cloning.
type Renderer struct {
	tmpl *template.Template
}

func funcs() template.FuncMap {
	return template.FuncMap{
		"statuses": Statuses,
		"badgeFor": func(s Status) Badge {
			b, _ := BadgeFor(s)

			return b
		},
	}
}

// NewRenderer parses the embedded templates and components.
func NewRenderer() (*Renderer, error) {
	return newRenderer(templateFS)
}

func newRenderer(fsys fs.FS) (*Renderer, error) {
	tmpl, err := template.New("paddock").Funcs(funcs()).ParseFS(
		fsys, "components/*.tmpl", "templates/pages/*.tmpl",
	)
	if err != nil {
		return nil, err
	}

	return &Renderer{tmpl: tmpl}, nil
}

// Render writes the named page (e.g. "styleguide"); the page itself pulls in the
// layout partials. An unknown page name returns an error.
func (r *Renderer) Render(w io.Writer, page string, data any) error {
	return r.tmpl.ExecuteTemplate(w, page, data)
}

// RenderBadge writes the racing-flag badge for a status token — handy for HTMX
// partial responses that swap a single badge.
func (r *Renderer) RenderBadge(w io.Writer, status Status) error {
	badge, ok := BadgeFor(status)
	if !ok {
		return ErrUnknownStatus
	}

	return r.tmpl.ExecuteTemplate(w, "badge", badge)
}

// PageHandler returns an http.HandlerFunc that renders the named page. A render
// failure (e.g. an unknown page) becomes a 500.
func (r *Renderer) PageHandler(page string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		if err := r.Render(w, page, nil); err != nil {
			http.Error(w, "render error", http.StatusInternalServerError)
		}
	}
}

// StaticHandler serves the embedded static assets (CSS, fonts, JS) under
// /static/.
func StaticHandler() http.Handler {
	sub, _ := fs.Sub(staticFS, "static")

	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}
