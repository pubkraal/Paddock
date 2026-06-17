package httpx_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/pubkraal/paddock/internal/platform/httpx"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestServer_StartServeStop(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("paddock"))
	})

	srv := httpx.NewServer("web", "127.0.0.1:0", handler, silentLogger())

	if srv.Name() != "web" {
		t.Errorf("Name() = %q, want web", srv.Name())
	}

	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	resp, err := http.Get("http://" + srv.Addr() + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "paddock" {
		t.Errorf("body = %q, want paddock", body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestServer_AddrBeforeStart(t *testing.T) {
	t.Parallel()

	srv := httpx.NewServer("web", "127.0.0.1:8080", http.NotFoundHandler(), silentLogger())

	if srv.Addr() != "127.0.0.1:8080" {
		t.Errorf("Addr() = %q, want the configured addr before Start", srv.Addr())
	}
}

type fakeListener struct {
	acceptErr error
}

func (l *fakeListener) Accept() (net.Conn, error) { return nil, l.acceptErr }
func (l *fakeListener) Close() error              { return nil }
func (l *fakeListener) Addr() net.Addr            { return &net.TCPAddr{} }

func TestServer_ServeLogsUnexpectedError(t *testing.T) {
	t.Parallel()

	rec := &recordingHandler{}
	srv := httpx.NewServer("web", "127.0.0.1:0", http.NotFoundHandler(), slog.New(rec))

	srv.Serve(&fakeListener{acceptErr: errors.New("accept exploded")})

	if rec.last() == nil {
		t.Error("an unexpected serve error should be logged")
	}
}

func TestServer_ServeIgnoresServerClosed(t *testing.T) {
	t.Parallel()

	rec := &recordingHandler{}
	srv := httpx.NewServer("web", "127.0.0.1:0", http.NotFoundHandler(), slog.New(rec))

	srv.Serve(&fakeListener{acceptErr: http.ErrServerClosed})

	if rec.last() != nil {
		t.Error("ErrServerClosed is a normal shutdown and must not be logged as an error")
	}
}

func TestServer_StartBindError(t *testing.T) {
	t.Parallel()

	// Occupy a port, then try to start a server on the same address.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	srv := httpx.NewServer("web", ln.Addr().String(), http.NotFoundHandler(), silentLogger())

	if err := srv.Start(context.Background()); err == nil {
		_ = srv.Stop(context.Background())
		t.Fatal("expected bind error on an occupied port, got nil")
	}
}
