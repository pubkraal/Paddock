package catalog_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/julienschmidt/httprouter"
	"github.com/pubkraal/paddock/internal/catalog"
	"github.com/pubkraal/paddock/internal/identity"
	"github.com/pubkraal/paddock/internal/platform/tabular"
	"github.com/pubkraal/paddock/web"
)

// mockSetupService scripts the catalog service for handler tests.
type mockSetupService struct {
	event      catalog.Event
	events     []catalog.Event
	detail     catalog.EventDetail
	entry      catalog.EntryPreview
	accred     catalog.AccreditationResult
	tier       catalog.Tier
	createErr  error
	goLiveErr  error
	listErr    error
	detailErr  error
	entryErr   error
	accredErr  error
	tierErr    error
	importedAs string
}

func (m *mockSetupService) CreateEventFromTemplate(context.Context, string, catalog.CreateEventInput) (catalog.Event, error) {
	return m.event, m.createErr
}

func (m *mockSetupService) GoLive(context.Context, string, string) error { return m.goLiveErr }

func (m *mockSetupService) ListEvents(context.Context, string) ([]catalog.Event, error) {
	return m.events, m.listErr
}

func (m *mockSetupService) EventDetail(context.Context, string, string) (catalog.EventDetail, error) {
	return m.detail, m.detailErr
}

func (m *mockSetupService) ImportEntryList(
	_ context.Context, _, _, filename string, _ tabular.Sheet,
) (catalog.EntryPreview, error) {
	m.importedAs = filename

	return m.entry, m.entryErr
}

func (m *mockSetupService) ImportAccreditation(
	context.Context, string, string, tabular.Sheet,
) (catalog.AccreditationResult, error) {
	return m.accred, m.accredErr
}

func (m *mockSetupService) ConsumerTier(context.Context, string, string) (catalog.Tier, error) {
	return m.tier, m.tierErr
}

// fakeGetter resolves any session id to a fixed session.
type fakeGetter struct{ sess identity.Session }

func (f fakeGetter) Get(context.Context, string) (identity.Session, error) {
	return f.sess, nil
}

func newHandler(t *testing.T, svc *mockSetupService) *catalog.Handler {
	t.Helper()

	renderer, err := web.NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	return catalog.NewHandler(catalog.HandlerConfig{
		Service:  svc,
		Renderer: renderer,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func newRouter(t *testing.T, svc *mockSetupService, sess identity.Session) http.Handler {
	t.Helper()

	h := newHandler(t, svc)

	auth := identity.Authenticate(fakeGetter{sess: sess})

	router := httprouter.New()
	router.Handler("GET", "/admin", auth(h.Dashboard()))
	router.Handler("GET", "/admin/events/new", auth(h.NewEvent()))
	router.Handler("POST", "/admin/events", auth(h.CreateEvent()))
	router.Handler("POST", "/admin/events/:id/entry-list", auth(h.UploadEntryList()))
	router.Handler("POST", "/admin/events/:id/accreditation", auth(h.UploadAccreditation()))
	router.Handler("POST", "/admin/events/:id/go-live", auth(h.GoLive()))
	router.Handler("GET", "/portal", auth(h.Portal()))
	router.Handler("GET", "/admin/archive", auth(h.Archive()))

	return router
}

func adminSession() identity.Session {
	return identity.Session{UserID: "user-1", OrgID: "org-1", Role: identity.RolePressOfficer, Kind: identity.KindAdmin}
}

func do(t *testing.T, router http.Handler, method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, body)
	req.AddCookie(&http.Cookie{Name: identity.SessionCookie, Value: "sess-1"})

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	return rr
}

func multipartFile(t *testing.T, filename, content string) (*bytes.Buffer, string) {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}

	if _, err := io.WriteString(fw, content); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	return &buf, w.FormDataContentType()
}

func TestHandler_Dashboard(t *testing.T) {
	t.Parallel()

	svc := &mockSetupService{events: []catalog.Event{{ID: "e1", Name: "24H Nürburgring", Status: catalog.EventLive}}}
	rr := do(t, newRouter(t, svc, adminSession()), http.MethodGet, "/admin", nil, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "24H Nürburgring") {
		t.Error("dashboard does not list the event")
	}
}

func TestHandler_DashboardListError(t *testing.T) {
	t.Parallel()

	svc := &mockSetupService{listErr: errBoomSvc}
	rr := do(t, newRouter(t, svc, adminSession()), http.MethodGet, "/admin", nil, "")

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

func TestHandler_NewEvent(t *testing.T) {
	t.Parallel()

	rr := do(t, newRouter(t, &mockSetupService{}, adminSession()), http.MethodGet, "/admin/events/new", nil, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	body := rr.Body.String()
	for _, want := range []string{"Sprint weekend", "Endurance", "Rally"} {
		if !strings.Contains(body, want) {
			t.Errorf("wizard missing template %q", want)
		}
	}
}

func TestHandler_CreateEvent(t *testing.T) {
	t.Parallel()

	svc := &mockSetupService{
		event: catalog.Event{ID: "event-1", Name: "24H"},
		detail: catalog.EventDetail{
			Event:    catalog.Event{ID: "event-1", Name: "24H"},
			Sessions: []catalog.Session{{Name: "FP", Type: catalog.SessionPractice}},
		},
	}

	form := strings.NewReader("template=sprint&event_name=24H")
	rr := do(t, newRouter(t, svc, adminSession()), http.MethodPost, "/admin/events", form,
		"application/x-www-form-urlencoded")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "FP") {
		t.Error("response missing scaffolded session")
	}
}

func TestHandler_CreateEventError(t *testing.T) {
	t.Parallel()

	svc := &mockSetupService{createErr: catalog.ErrUnknownTemplate}
	rr := do(t, newRouter(t, svc, adminSession()), http.MethodPost, "/admin/events",
		strings.NewReader("template=nope"), "application/x-www-form-urlencoded")

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

func TestHandler_CreateEventDetailError(t *testing.T) {
	t.Parallel()

	svc := &mockSetupService{event: catalog.Event{ID: "e1"}, detailErr: errBoomSvc}
	rr := do(t, newRouter(t, svc, adminSession()), http.MethodPost, "/admin/events",
		strings.NewReader("template=sprint&event_name=E"), "application/x-www-form-urlencoded")

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

func TestHandler_UploadEntryList(t *testing.T) {
	t.Parallel()

	svc := &mockSetupService{
		detail: catalog.EventDetail{Event: catalog.Event{ID: "event-1"}},
		entry:  catalog.EntryPreview{Entries: []catalog.Entry{{CarNo: "72", Team: "AMG"}}},
	}

	body, ct := multipartFile(t, "entrylist.csv", "Car,Team\n72,AMG\n")
	rr := do(t, newRouter(t, svc, adminSession()), http.MethodPost, "/admin/events/event-1/entry-list", body, ct)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	if svc.importedAs != "entrylist.csv" {
		t.Errorf("imported filename = %q, want entrylist.csv", svc.importedAs)
	}

	if !strings.Contains(rr.Body.String(), "1 cars") {
		t.Error("response missing entry tally")
	}
}

func TestHandler_UploadEntryListUnsupportedType(t *testing.T) {
	t.Parallel()

	body, ct := multipartFile(t, "list.pdf", "nope")
	rr := do(t, newRouter(t, &mockSetupService{}, adminSession()), http.MethodPost,
		"/admin/events/event-1/entry-list", body, ct)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (error fragment)", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "Import failed") {
		t.Error("expected the import-error fragment")
	}
}

func TestHandler_UploadEntryListImportError(t *testing.T) {
	t.Parallel()

	svc := &mockSetupService{entryErr: errBoomSvc}
	body, ct := multipartFile(t, "entrylist.csv", "Car\n72\n")
	rr := do(t, newRouter(t, svc, adminSession()), http.MethodPost, "/admin/events/event-1/entry-list", body, ct)

	if !strings.Contains(rr.Body.String(), "Import failed") {
		t.Error("expected import-error fragment on service error")
	}
}

func TestHandler_UploadEntryListNoFile(t *testing.T) {
	t.Parallel()

	// A valid multipart body whose only field is not "file": ParseMultipartForm
	// succeeds, FormFile("file") fails.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if err := w.WriteField("other", "value"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	rr := do(t, newRouter(t, &mockSetupService{}, adminSession()), http.MethodPost,
		"/admin/events/event-1/entry-list", &buf, w.FormDataContentType())

	if !strings.Contains(rr.Body.String(), "Import failed") {
		t.Error("expected import-error fragment when no file field is present")
	}
}

func TestHandler_UploadAccreditation(t *testing.T) {
	t.Parallel()

	svc := &mockSetupService{
		detail: catalog.EventDetail{Event: catalog.Event{ID: "event-1"}},
		accred: catalog.AccreditationResult{
			AccreditationPreview: catalog.AccreditationPreview{TierCounts: map[catalog.Tier]int{catalog.TierMedia: 2}},
			Invited:              2,
		},
	}

	body, ct := multipartFile(t, "accred.csv", "Name,Email,Tier\nS,a@b.test,media\n")
	rr := do(t, newRouter(t, svc, adminSession()), http.MethodPost, "/admin/events/event-1/accreditation", body, ct)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "2 magic-link invites enqueued") {
		t.Error("response missing invite tally")
	}
}

func TestHandler_UploadAccreditationErrors(t *testing.T) {
	t.Parallel()

	t.Run("bad file", func(t *testing.T) {
		t.Parallel()

		body, ct := multipartFile(t, "a.txt", "x")
		rr := do(t, newRouter(t, &mockSetupService{}, adminSession()), http.MethodPost,
			"/admin/events/event-1/accreditation", body, ct)
		if !strings.Contains(rr.Body.String(), "Import failed") {
			t.Error("expected import-error fragment")
		}
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		svc := &mockSetupService{accredErr: errBoomSvc}
		body, ct := multipartFile(t, "a.csv", "Name,Email,Tier\n")
		rr := do(t, newRouter(t, svc, adminSession()), http.MethodPost,
			"/admin/events/event-1/accreditation", body, ct)
		if !strings.Contains(rr.Body.String(), "Import failed") {
			t.Error("expected import-error fragment")
		}
	})
}

func TestHandler_GoLive(t *testing.T) {
	t.Parallel()

	rr := do(t, newRouter(t, &mockSetupService{}, adminSession()), http.MethodPost,
		"/admin/events/event-1/go-live", nil, "")

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rr.Code)
	}

	if loc := rr.Header().Get("Location"); loc != "/admin" {
		t.Errorf("Location = %q, want /admin", loc)
	}
}

func TestHandler_GoLiveError(t *testing.T) {
	t.Parallel()

	svc := &mockSetupService{goLiveErr: errBoomSvc}
	rr := do(t, newRouter(t, svc, adminSession()), http.MethodPost, "/admin/events/event-1/go-live", nil, "")

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

func TestHandler_PortalDispatchByTier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier catalog.Tier
		want string
	}{
		{catalog.TierMedia, "Paddock Studio"},
		{catalog.TierSponsor, "Paddock Partners"},
		{catalog.TierTeam, "Paddock Team"},
		{catalog.TierInternal, "Paddock Studio"},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			t.Parallel()

			sess := identity.Session{UserID: "u1", OrgID: "org-1", Role: identity.RoleConsumer, Kind: identity.KindConsumer}
			svc := &mockSetupService{tier: tt.tier}
			rr := do(t, newRouter(t, svc, sess), http.MethodGet, "/portal", nil, "")

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}

			if !strings.Contains(rr.Body.String(), tt.want) {
				t.Errorf("portal for %s missing %q", tt.tier, tt.want)
			}
		})
	}
}

func TestHandler_PortalTierError(t *testing.T) {
	t.Parallel()

	sess := identity.Session{UserID: "u1", OrgID: "org-1", Role: identity.RoleConsumer}
	svc := &mockSetupService{tierErr: errBoomSvc}
	rr := do(t, newRouter(t, svc, sess), http.MethodGet, "/portal", nil, "")

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

func TestHandler_Archive(t *testing.T) {
	t.Parallel()

	sess := identity.Session{UserID: "u1", OrgID: "org-asn", Role: identity.RolePressOfficer}
	rr := do(t, newRouter(t, &mockSetupService{}, sess), http.MethodGet, "/admin/archive", nil, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "Paddock Archive") {
		t.Error("archive dashboard did not render")
	}
}

// TestHandler_Unauthenticated calls the gated handlers without an identity in
// context (bypassing Authenticate) to cover their defensive redirect.
func TestHandler_Unauthenticated(t *testing.T) {
	t.Parallel()

	h := newHandler(t, &mockSetupService{})

	gated := map[string]http.HandlerFunc{
		"dashboard":  h.Dashboard(),
		"create":     h.CreateEvent(),
		"entry-list": h.UploadEntryList(),
		"accred":     h.UploadAccreditation(),
		"go-live":    h.GoLive(),
		"portal":     h.Portal(),
	}

	for name, handler := range gated {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rr := httptest.NewRecorder()
			handler(rr, httptest.NewRequest(http.MethodGet, "/", nil))

			if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/login" {
				t.Errorf("%s without identity = %d %q, want 303 /login", name, rr.Code, rr.Header().Get("Location"))
			}
		})
	}
}

// failingRenderer always errors, to cover the render 500 branch.
type failingRenderer struct{}

func (failingRenderer) Render(io.Writer, string, any) error { return errBoomSvc }

func TestHandler_RenderError(t *testing.T) {
	t.Parallel()

	h := catalog.NewHandler(catalog.HandlerConfig{
		Service:  &mockSetupService{},
		Renderer: failingRenderer{},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	rr := httptest.NewRecorder()
	h.NewEvent()(rr, httptest.NewRequest(http.MethodGet, "/admin/events/new", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 on render failure", rr.Code)
	}
}

func TestHandler_UploadParseError(t *testing.T) {
	t.Parallel()

	// A .xlsx extension is accepted by ParserFor, but the content is not a real
	// workbook, so parser.Parse fails.
	body, ct := multipartFile(t, "list.xlsx", "not a workbook")
	rr := do(t, newRouter(t, &mockSetupService{}, adminSession()), http.MethodPost,
		"/admin/events/event-1/entry-list", body, ct)

	if !strings.Contains(rr.Body.String(), "Import failed") {
		t.Error("expected import-error fragment on parse failure")
	}
}

func TestHandler_UploadMalformedMultipart(t *testing.T) {
	t.Parallel()

	// A multipart content type with no boundary parameter fails ParseMultipartForm.
	rr := do(t, newRouter(t, &mockSetupService{}, adminSession()), http.MethodPost,
		"/admin/events/event-1/entry-list", strings.NewReader("whatever"), "multipart/form-data")

	if !strings.Contains(rr.Body.String(), "Import failed") {
		t.Error("expected import-error fragment on malformed multipart")
	}
}

func TestHandler_PortalUnknownTierDefaults(t *testing.T) {
	t.Parallel()

	sess := identity.Session{UserID: "u1", OrgID: "org-1", Role: identity.RoleConsumer}
	svc := &mockSetupService{tier: catalog.Tier("mystery")}
	rr := do(t, newRouter(t, svc, sess), http.MethodGet, "/portal", nil, "")

	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Paddock Studio") {
		t.Errorf("unknown tier should default to photographer; got %d", rr.Code)
	}
}
