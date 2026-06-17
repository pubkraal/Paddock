// Command worker drains the River job queue (ADR-0001, ADR-0005): ingest
// processing, renditions, watermarks, ZIPs, email, embargo lifts. Phase 0 boots
// the consumer loop with an empty worker registry; job types are registered by
// later phases.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/pubkraal/paddock/internal/platform/config"
	"github.com/pubkraal/paddock/internal/platform/postgres"
	"github.com/pubkraal/paddock/internal/platform/queue"
	"github.com/pubkraal/paddock/internal/platform/runtime"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("worker exited with error", slog.Any("err", err))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.LoadWorker(os.Getenv)
	if err != nil {
		return err
	}

	ctx := context.Background()

	pool, err := postgres.Open(ctx, cfg.Postgres.URL)
	if err != nil {
		return err
	}
	defer func() { _ = pool.Close() }()

	client, err := queue.NewWorkerClient(pool.SQL(), queue.DefaultWorkers(), cfg.Concurrency, logger)
	if err != nil {
		return err
	}

	service := queue.NewService("worker", client)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	return runtime.Run(ctx, logger, cfg.ShutdownTimeout, sig, service)
}
