package httpx_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/pubkraal/paddock/internal/platform/httpx"
)

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	t.Run("always-on headers, no HSTS when insecure", func(t *testing.T) {
		t.Parallel()

		rr := httptest.NewRecorder()
		httpx.SecurityHeaders(false)(next).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		want := map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "strict-origin-when-cross-origin",
		}
		for k, v := range want {
			if got := rr.Header().Get(k); got != v {
				t.Errorf("%s = %q, want %q", k, got, v)
			}
		}

		if csp := rr.Header().Get("Content-Security-Policy"); csp == "" || !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Errorf("CSP = %q, want a policy with frame-ancestors 'none'", csp)
		}

		if hsts := rr.Header().Get("Strict-Transport-Security"); hsts != "" {
			t.Errorf("HSTS = %q, want empty when insecure", hsts)
		}
	})

	t.Run("HSTS when secure", func(t *testing.T) {
		t.Parallel()

		rr := httptest.NewRecorder()
		httpx.SecurityHeaders(true)(next).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		if hsts := rr.Header().Get("Strict-Transport-Security"); hsts == "" {
			t.Error("HSTS header missing when secure")
		}
	})
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	httpx.Healthz()(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
}

func TestReadyz_AllPass(t *testing.T) {
	t.Parallel()

	checks := map[string]httpx.Check{
		"postgres": func(context.Context) error { return nil },
		"redis":    func(context.Context) error { return nil },
	}

	rr := httptest.NewRecorder()
	httpx.Readyz(checks)(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

func TestReadyz_OneFails(t *testing.T) {
	t.Parallel()

	errDown := errors.New("connection refused")
	checks := map[string]httpx.Check{
		"postgres": func(context.Context) error { return nil },
		"redis":    func(context.Context) error { return errDown },
	}

	rr := httptest.NewRecorder()
	httpx.Readyz(checks)(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}

	if body := rr.Body.String(); !contains(body, "redis") {
		t.Errorf("body %q should name the failed check", body)
	}
}

func TestRequestID_Generated(t *testing.T) {
	t.Parallel()

	var got string

	h := httpx.RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = httpx.RequestIDFromContext(r.Context())
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if got == "" {
		t.Fatal("request id not present in context")
	}

	if rr.Header().Get("X-Request-Id") != got {
		t.Errorf("response header %q != context id %q", rr.Header().Get("X-Request-Id"), got)
	}
}

func TestRequestID_Propagated(t *testing.T) {
	t.Parallel()

	const supplied = "client-supplied-id"

	var got string

	h := httpx.RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = httpx.RequestIDFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", supplied)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got != supplied {
		t.Errorf("request id = %q, want propagated %q", got, supplied)
	}
}

func TestRequestIDFromContext_Absent(t *testing.T) {
	t.Parallel()

	if id := httpx.RequestIDFromContext(context.Background()); id != "" {
		t.Errorf("id = %q, want empty for a context without one", id)
	}
}

func TestRecovery_PanicBecomes500(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	h := httpx.Recovery(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rr := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped recovery middleware: %v", r)
		}
	}()

	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestLogging_EmitsRecordWithStatus(t *testing.T) {
	t.Parallel()

	rec := &recordingHandler{}
	logger := slog.New(rec)

	h := httpx.Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/curate", nil))

	if rr.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418 (passed through)", rr.Code)
	}

	r := rec.last()
	if r == nil {
		t.Fatal("no log record emitted")
	}

	if status, ok := r["status"]; !ok || status != int64(http.StatusTeapot) {
		t.Errorf("log status attr = %v, want 418", status)
	}

	if path, ok := r["path"]; !ok || path != "/curate" {
		t.Errorf("log path attr = %v, want /curate", path)
	}
}

func TestLogging_DefaultsTo200OnWrite(t *testing.T) {
	t.Parallel()

	rec := &recordingHandler{}
	logger := slog.New(rec)

	h := httpx.Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Body.String() != "ok" {
		t.Fatalf("body = %q, want ok", rr.Body.String())
	}

	r := rec.last()
	if r == nil || r["status"] != int64(http.StatusOK) {
		t.Errorf("log status = %v, want 200 default when only Write is called", r["status"])
	}
}

func TestChain_OrdersOutsideIn(t *testing.T) {
	t.Parallel()

	var order []string

	mw := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}

	final := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		order = append(order, "handler")
	})

	h := httpx.Chain(final, mw("first"), mw("second"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"first", "second", "handler"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}

	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d] = %q, want %q", i, order[i], want[i])
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}

	return false
}

type recordingHandler struct {
	mu      sync.Mutex
	records []map[string]any
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	m := map[string]any{"msg": r.Message}
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.Any()

		return true
	})
	h.records = append(h.records, m)

	return nil
}

func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

func (h *recordingHandler) WithGroup(_ string) slog.Handler { return h }

func (h *recordingHandler) last() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.records) == 0 {
		return nil
	}

	return h.records[len(h.records)-1]
}
