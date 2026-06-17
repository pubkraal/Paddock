package web_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/pubkraal/paddock/web"
)

func newRenderer(t *testing.T) *web.Renderer {
	t.Helper()

	r, err := web.NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	return r
}

func TestBadgeFor_MapsStateToFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status    web.Status
		wantLabel string
		wantFlag  string
	}{
		{web.StatusRaw, "RAW", "neutral"},
		{web.StatusSelect, "SELECT", "yellow"},
		{web.StatusPublished, "PUBLISHED", "green"},
		{web.StatusEmbargoed, "EMBARGO", "red"},
		{web.StatusKilled, "KILLED", "red"},
		{web.StatusArchived, "CLOSED", "chequered"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()

			badge, ok := web.BadgeFor(tt.status)
			if !ok {
				t.Fatalf("BadgeFor(%q) not found", tt.status)
			}

			if badge.Label != tt.wantLabel {
				t.Errorf("Label = %q, want %q", badge.Label, tt.wantLabel)
			}

			if badge.Flag != tt.wantFlag {
				t.Errorf("Flag = %q, want %q", badge.Flag, tt.wantFlag)
			}
		})
	}
}

func TestBadgeFor_UnknownStatus(t *testing.T) {
	t.Parallel()

	if _, ok := web.BadgeFor(web.Status("bogus")); ok {
		t.Error("BadgeFor reported an unknown status as found")
	}
}

func TestRenderBadge_ProducesFlagPill(t *testing.T) {
	t.Parallel()

	r := newRenderer(t)

	var buf bytes.Buffer
	if err := r.RenderBadge(&buf, web.StatusPublished); err != nil {
		t.Fatalf("RenderBadge: %v", err)
	}

	html := buf.String()

	for _, want := range []string{"badge--green", "PUBLISHED", "badge__dot"} {
		if !strings.Contains(html, want) {
			t.Errorf("badge HTML %q missing %q", html, want)
		}
	}
}

func TestRenderBadge_UnknownStatus(t *testing.T) {
	t.Parallel()

	r := newRenderer(t)

	if err := r.RenderBadge(&bytes.Buffer{}, web.Status("bogus")); err == nil {
		t.Fatal("expected error rendering an unknown status, got nil")
	}
}

func TestRender_StyleguideShowsEveryFlag(t *testing.T) {
	t.Parallel()

	r := newRenderer(t)

	var buf bytes.Buffer
	if err := r.Render(&buf, "styleguide", nil); err != nil {
		t.Fatalf("Render: %v", err)
	}

	html := buf.String()

	if !strings.Contains(html, "<!doctype html>") {
		t.Error("styleguide is not a full page (base layout missing)")
	}

	for _, label := range []string{"RAW", "SELECT", "PUBLISHED", "EMBARGO", "KILLED", "CLOSED"} {
		if !strings.Contains(html, label) {
			t.Errorf("styleguide missing badge label %q", label)
		}
	}

	for _, ref := range []string{"Archivo", "JetBrains Mono", "/static/css/"} {
		if !strings.Contains(html, ref) {
			t.Errorf("styleguide missing design-system reference %q", ref)
		}
	}
}

func TestRender_UnknownPage(t *testing.T) {
	t.Parallel()

	r := newRenderer(t)

	if err := r.Render(&bytes.Buffer{}, "does-not-exist", nil); err == nil {
		t.Fatal("expected error for an unknown page, got nil")
	}
}

func TestPageHandler_RendersPage(t *testing.T) {
	t.Parallel()

	r := newRenderer(t)

	rr := httptest.NewRecorder()
	r.PageHandler("styleguide").ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/_styleguide", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}

	if !strings.Contains(rr.Body.String(), "PUBLISHED") {
		t.Error("rendered page missing badge content")
	}
}

func TestPageHandler_RenderErrorIs500(t *testing.T) {
	t.Parallel()

	r := newRenderer(t)

	rr := httptest.NewRecorder()
	r.PageHandler("does-not-exist").ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for an unknown page", rr.Code)
	}
}

func TestNewRenderer_ParseError(t *testing.T) {
	t.Parallel()

	// A filesystem with no component templates cannot be parsed.
	if _, err := web.NewRendererFS(fstest.MapFS{}); err == nil {
		t.Fatal("expected parse error for an empty filesystem, got nil")
	}
}

func TestStaticHandler_ServesEmbeddedAsset(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	web.StaticHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), ".badge") {
		t.Error("served CSS does not look like app.css")
	}
}
