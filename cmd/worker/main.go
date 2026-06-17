// Command worker drains the River job queue (ADR-0001, ADR-0005): ingest
// processing, renditions, watermarks, ZIPs, email, embargo lifts. Phase 2 adds
// the first real job: accreditation invites, which issue a consumer-grant magic
// link and email it (ADR-0016).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pubkraal/paddock/internal/identity"
	"github.com/pubkraal/paddock/internal/invite"
	"github.com/pubkraal/paddock/internal/platform/config"
	"github.com/pubkraal/paddock/internal/platform/mailer"
	"github.com/pubkraal/paddock/internal/platform/postgres"
	"github.com/pubkraal/paddock/internal/platform/queue"
	"github.com/pubkraal/paddock/internal/platform/redis"
	"github.com/pubkraal/paddock/internal/platform/runtime"
	"github.com/riverqueue/river"
)

// inviteSendTimeout bounds how long an invite email send may block a worker job.
const inviteSendTimeout = 10 * time.Second

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

	rdb := redis.Open(cfg.Redis)
	defer func() { _ = rdb.Close() }()

	mail := mailer.NewSMTPMailer(cfg.Mailer.SMTPAddr, cfg.Mailer.From)
	tokens := identity.NewTokenStore(rdb, cfg.Auth.MagicLinkTTL)
	sessions := identity.NewSessionStore(rdb, cfg.Auth.SessionTTL)
	idRepo := identity.NewRepository(pool)
	linkMailer := identity.NewLinkMailer(mail, inviteSendTimeout)
	idSvc := identity.NewService(idRepo, tokens, sessions, linkMailer, cfg.Auth.BaseURL, time.Now)

	workers := queue.DefaultWorkers()
	river.AddWorker(workers, invite.NewWorker(idSvc))

	client, err := queue.NewWorkerClient(pool.SQL(), workers, cfg.Concurrency, logger)
	if err != nil {
		return err
	}

	service := queue.NewService("worker", client)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	return runtime.Run(ctx, logger, cfg.ShutdownTimeout, sig, service)
}
