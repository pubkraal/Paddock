package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 120 * time.Second
)

// Server is an http.Server wrapped to satisfy the runtime.Service lifecycle:
// Start binds the listener (surfacing bind errors synchronously) and serves in
// the background; Stop drains connections gracefully within the given context.
type Server struct {
	name   string
	srv    *http.Server
	logger *slog.Logger
	ln     net.Listener
}

// NewServer builds a server for the given address and handler.
func NewServer(name, addr string, handler http.Handler, logger *slog.Logger) *Server {
	return &Server{
		name:   name,
		logger: logger,
		srv: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
			IdleTimeout:       idleTimeout,
		},
	}
}

// Name identifies the server in lifecycle logs.
func (s *Server) Name() string {
	return s.name
}

// Addr returns the actual bound address (useful when the configured port is 0).
func (s *Server) Addr() string {
	if s.ln == nil {
		return s.srv.Addr
	}

	return s.ln.Addr().String()
}

// Start binds the listener and begins serving in a background goroutine. A bind
// failure is returned synchronously so startup can abort.
func (s *Server) Start(_ context.Context) error {
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return err
	}

	s.ln = ln

	go s.serve(ln)

	return nil
}

func (s *Server) serve(ln net.Listener) {
	if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Error("http serve error", slog.String("server", s.name), slog.Any("err", err))
	}
}

// Stop gracefully shuts the server down within ctx's deadline.
func (s *Server) Stop(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
