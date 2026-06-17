// Command ftp-gateway is the camera FTP/SFTP ingest endpoint (ADR-0001,
// ADR-0004). Phase 0 is a skeleton: it binds its port, rejects every connection,
// and shuts down gracefully. The real SFTP/FTPS server and per-photographer
// credentials land in Phase 4 (open decision O3 on the server library).
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/pubkraal/paddock/internal/platform/config"
	"github.com/pubkraal/paddock/internal/platform/runtime"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("ftp-gateway exited with error", slog.Any("err", err))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.LoadFTPGateway(os.Getenv)
	if err != nil {
		return err
	}

	ctx := context.Background()
	gateway := newRejectGateway(cfg.Addr, logger)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	return runtime.Run(ctx, logger, cfg.ShutdownTimeout, sig, gateway)
}

// rejectGateway binds a TCP port and closes every connection immediately — the
// Phase 0 placeholder for the real ingest server.
type rejectGateway struct {
	addr   string
	logger *slog.Logger
	ln     net.Listener
}

func newRejectGateway(addr string, logger *slog.Logger) *rejectGateway {
	return &rejectGateway{addr: addr, logger: logger}
}

func (g *rejectGateway) Name() string { return "ftp-gateway" }

func (g *rejectGateway) Start(_ context.Context) error {
	ln, err := net.Listen("tcp", g.addr)
	if err != nil {
		return err
	}

	g.ln = ln

	go g.acceptLoop()

	return nil
}

func (g *rejectGateway) acceptLoop() {
	for {
		conn, err := g.ln.Accept()
		if err != nil {
			return // listener closed during shutdown
		}

		_ = conn.Close()
	}
}

func (g *rejectGateway) Stop(_ context.Context) error {
	if g.ln == nil {
		return nil
	}

	return g.ln.Close()
}
