// Package queue wraps River (ADR-0005), the Postgres-backed job queue. It uses
// River's database/sql driver so that job enqueue shares the very same *sql.Tx
// as the domain write that triggers it (ADR-0009) — the transactional-enqueue
// guarantee that eliminates orphaned assets. The web and ftp-gateway tiers use
// an insert-only client; cmd/worker uses a consuming client wrapped as a
// runtime.Service.
package queue

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
)

// Workers returns an empty River worker registry. Job workers are added to it as
// later phases introduce job types (renditions, embargo lifts, email).
func Workers() *river.Workers {
	return river.NewWorkers()
}

// NoopArgs is a placeholder job type. River refuses to start a consuming client
// with an empty registry, so Phase 0 registers this no-op to make the worker a
// valid, bootable consumer before real job types exist (Phase 3 onward).
type NoopArgs struct{}

// Kind identifies the job type in the River tables.
func (NoopArgs) Kind() string { return "noop" }

// NoopWorker does nothing; it exists only to populate the registry.
type NoopWorker struct {
	river.WorkerDefaults[NoopArgs]
}

// Work is a no-op.
func (*NoopWorker) Work(context.Context, *river.Job[NoopArgs]) error { return nil }

// DefaultWorkers returns a registry seeded with the no-op worker so cmd/worker
// can boot a valid consumer in Phase 0.
func DefaultWorkers() *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker(workers, &NoopWorker{})

	return workers
}

// NewInsertClient builds an insert-only client for the web and ftp-gateway tiers
// to enqueue jobs transactionally. It does not fetch or work jobs.
func NewInsertClient(db *sql.DB, logger *slog.Logger) (*river.Client[*sql.Tx], error) {
	return river.NewClient(riverdatabasesql.New(db), &river.Config{Logger: logger})
}

// NewWorkerClient builds the consuming client for cmd/worker, bounding the
// default queue's concurrency to the configured worker count.
func NewWorkerClient(
	db *sql.DB, workers *river.Workers, concurrency int, logger *slog.Logger,
) (*river.Client[*sql.Tx], error) {
	return river.NewClient(riverdatabasesql.New(db), &river.Config{
		Logger:  logger,
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: concurrency},
		},
	})
}

// lifecycle is the start/stop surface of a River client, defined here so Service
// can be unit-tested with a fake.
type lifecycle interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Service adapts a River client to the runtime.Service lifecycle.
type Service struct {
	name string
	lc   lifecycle
}

// NewService wraps a River client (or any start/stop lifecycle) as a Service.
func NewService(name string, lc lifecycle) *Service {
	return &Service{name: name, lc: lc}
}

// Name identifies the worker in lifecycle logs.
func (s *Service) Name() string {
	return s.name
}

// Start begins the River fetching/working loops.
func (s *Service) Start(ctx context.Context) error {
	return s.lc.Start(ctx)
}

// Stop gracefully drains in-flight jobs within ctx's deadline.
func (s *Service) Stop(ctx context.Context) error {
	return s.lc.Stop(ctx)
}
