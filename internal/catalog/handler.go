package catalog

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/pubkraal/paddock/internal/identity"
	"github.com/pubkraal/paddock/internal/platform/tabular"
)

// maxUploadBytes bounds a roster upload so a pathological file cannot exhaust
// memory; entry lists and accreditation rosters are kilobytes, not megabytes.
const maxUploadBytes = 8 << 20 // 8 MiB

// The Handler depends on these narrow interfaces, defined at the consumer.
type (
	setupService interface {
		CreateEventFromTemplate(ctx context.Context, orgID string, in CreateEventInput) (Event, error)
		GoLive(ctx context.Context, orgID, eventID string) error
		ListEvents(ctx context.Context, orgID string) ([]Event, error)
		EventDetail(ctx context.Context, orgID, eventID string) (EventDetail, error)
		ImportEntryList(ctx context.Context, orgID, eventID, filename string, sheet tabular.Sheet) (EntryPreview, error)
		ImportAccreditation(ctx context.Context, orgID, eventID string, sheet tabular.Sheet) (AccreditationResult, error)
		ConsumerTier(ctx context.Context, orgID, userID string) (Tier, error)
	}

	renderer interface {
		Render(w io.Writer, page string, data any) error
	}
)

// HandlerConfig wires a Handler.
type HandlerConfig struct {
	Service  setupService
	Renderer renderer
	Logger   *slog.Logger
}

// Handler serves the event-setup wizard, the roster imports, and the role
// dashboards (PLAN §6).
type Handler struct {
	svc      setupService
	renderer renderer
	logger   *slog.Logger
}

// NewHandler builds a Handler from its config.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{svc: cfg.Service, renderer: cfg.Renderer, logger: cfg.Logger}
}

type dashboardData struct {
	OrgID  string
	Role   string
	Events []Event
}

type wizardData struct {
	Templates []OnboardingTemplate
	CSRF      string
}

type eventStepData struct {
	Detail    EventDetail
	EntryList *EntryPreview
	Accred    *AccreditationResult
	CSRF      string
}

type portalData struct {
	OrgID string
	Tier  Tier
}

// Dashboard is the press-officer landing: the org's events and the setup CTA.
func (h *Handler) Dashboard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := identity.IdentityFromContext(r.Context())
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)

			return
		}

		events, err := h.svc.ListEvents(r.Context(), id.OrgID)
		if err != nil {
			h.fail(r.Context(), w, "list events", err)

			return
		}

		h.render(r.Context(), w, "dash_press", dashboardData{
			OrgID:  id.OrgID,
			Role:   string(id.Role),
			Events: events,
		})
	}
}

// NewEvent renders the event-setup wizard (template cards + the event form).
func (h *Handler) NewEvent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := identity.IdentityFromContext(r.Context())
		h.render(r.Context(), w, "setup_wizard", wizardData{Templates: Templates(), CSRF: id.CSRFToken})
	}
}

// CreateEvent scaffolds the event from the chosen template and swaps in the
// import step for it (HTMX), or renders the full step on a non-HTMX post.
func (h *Handler) CreateEvent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := identity.IdentityFromContext(r.Context())
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)

			return
		}

		year, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("season_year")))

		event, err := h.svc.CreateEventFromTemplate(r.Context(), id.OrgID, CreateEventInput{
			TemplateKey:      r.FormValue("template"),
			ChampionshipName: r.FormValue("championship"),
			SeasonName:       r.FormValue("season"),
			SeasonYear:       year,
			VenueName:        r.FormValue("venue"),
			VenueMapRef:      r.FormValue("circuit_map_ref"),
			EventName:        r.FormValue("event_name"),
		})
		if err != nil {
			h.fail(r.Context(), w, "create event", err)

			return
		}

		h.renderEventStep(r.Context(), w, id, event.ID, eventStepData{})
	}
}

// UploadEntryList imports an uploaded entry-list file and swaps in the result.
func (h *Handler) UploadEntryList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := identity.IdentityFromContext(r.Context())
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)

			return
		}

		eventID := httprouter.ParamsFromContext(r.Context()).ByName("id")

		filename, sheet, err := h.parseUpload(w, r)
		if err != nil {
			h.render(r.Context(), w, "import_error", err.Error())

			return
		}

		preview, err := h.svc.ImportEntryList(r.Context(), id.OrgID, eventID, filename, sheet)
		if err != nil {
			h.render(r.Context(), w, "import_error", err.Error())

			return
		}

		h.renderEventStep(r.Context(), w, id, eventID, eventStepData{EntryList: &preview})
	}
}

// UploadAccreditation imports an uploaded accreditation roster — provisioning
// consumers and enqueuing invites — and swaps in the result.
func (h *Handler) UploadAccreditation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := identity.IdentityFromContext(r.Context())
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)

			return
		}

		eventID := httprouter.ParamsFromContext(r.Context()).ByName("id")

		_, sheet, err := h.parseUpload(w, r)
		if err != nil {
			h.render(r.Context(), w, "import_error", err.Error())

			return
		}

		result, err := h.svc.ImportAccreditation(r.Context(), id.OrgID, eventID, sheet)
		if err != nil {
			h.render(r.Context(), w, "import_error", err.Error())

			return
		}

		h.renderEventStep(r.Context(), w, id, eventID, eventStepData{Accred: &result})
	}
}

// GoLive flips the event to live and returns to the dashboard.
func (h *Handler) GoLive() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := identity.IdentityFromContext(r.Context())
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)

			return
		}

		eventID := httprouter.ParamsFromContext(r.Context()).ByName("id")

		if err := h.svc.GoLive(r.Context(), id.OrgID, eventID); err != nil {
			h.fail(r.Context(), w, "go live", err)

			return
		}

		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	}
}

// Portal renders the consumer's tier-appropriate dashboard (PLAN §6): media →
// photographer, sponsor → sponsor, team → team-comms.
func (h *Handler) Portal() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := identity.IdentityFromContext(r.Context())
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)

			return
		}

		tier, err := h.svc.ConsumerTier(r.Context(), id.OrgID, id.UserID)
		if err != nil {
			h.fail(r.Context(), w, "consumer tier", err)

			return
		}

		h.render(r.Context(), w, dashboardForTier(tier), portalData{OrgID: id.OrgID, Tier: tier})
	}
}

// Archive renders the federation/archive dashboard.
func (h *Handler) Archive() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := identity.IdentityFromContext(r.Context())

		h.render(r.Context(), w, "dash_archive", portalData{OrgID: id.OrgID})
	}
}

func dashboardForTier(tier Tier) string {
	switch tier {
	case TierSponsor:
		return "dash_sponsor"
	case TierTeam:
		return "dash_teamcomms"
	case TierMedia, TierInternal:
		return "dash_photographer"
	default:
		return "dash_photographer"
	}
}

// renderEventStep loads the event's current detail and renders the import step,
// carrying any just-imported result for its fragment and the caller's CSRF token
// for the embedded forms.
func (h *Handler) renderEventStep(
	ctx context.Context, w http.ResponseWriter, id identity.Identity, eventID string, step eventStepData,
) {
	detail, err := h.svc.EventDetail(ctx, id.OrgID, eventID)
	if err != nil {
		h.fail(ctx, w, "event detail", err)

		return
	}

	step.Detail = detail
	step.CSRF = id.CSRFToken
	h.render(ctx, w, "wizard_event", step)
}

// parseUpload reads the single uploaded file, picks a parser by its filename,
// and returns the parsed sheet. w is threaded into MaxBytesReader so the server
// manages the connection when an upload exceeds the cap.
func (h *Handler) parseUpload(w http.ResponseWriter, r *http.Request) (string, tabular.Sheet, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		return "", tabular.Sheet{}, errUpload("could not read upload")
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return "", tabular.Sheet{}, errUpload("no file uploaded")
	}
	defer func() { _ = file.Close() }()

	parser, err := tabular.ParserFor(header.Filename)
	if err != nil {
		return "", tabular.Sheet{}, errUpload("unsupported file type — use CSV or XLSX")
	}

	sheet, err := parser.Parse(file)
	if err != nil {
		return "", tabular.Sheet{}, errUpload("could not parse file: " + err.Error())
	}

	return header.Filename, sheet, nil
}

type uploadError struct{ msg string }

func (e uploadError) Error() string { return e.msg }

func errUpload(msg string) error { return uploadError{msg: msg} }

func (h *Handler) render(ctx context.Context, w http.ResponseWriter, page string, data any) {
	var buf bytes.Buffer

	if err := h.renderer.Render(&buf, page, data); err != nil {
		h.logger.ErrorContext(ctx, "render page", "page", page, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (h *Handler) fail(ctx context.Context, w http.ResponseWriter, what string, err error) {
	h.logger.ErrorContext(ctx, what, "err", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
