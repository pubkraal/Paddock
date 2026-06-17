package queue_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/pubkraal/paddock/internal/platform/queue"
)

func logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestWorkers_Empty(t *testing.T) {
	t.Parallel()

	if queue.Workers() == nil {
		t.Fatal("Workers() returned nil")
	}
}

func TestNewInsertClient(t *testing.T) {
	t.Parallel()

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	client, err := queue.NewInsertClient(db, logger())
	if err != nil {
		t.Fatalf("NewInsertClient: %v", err)
	}

	if client == nil {
		t.Error("client is nil")
	}
}

func TestNoopWorker(t *testing.T) {
	t.Parallel()

	if (queue.NoopArgs{}).Kind() != "noop" {
		t.Errorf("Kind() = %q, want noop", (queue.NoopArgs{}).Kind())
	}

	if err := (&queue.NoopWorker{}).Work(context.Background(), nil); err != nil {
		t.Errorf("Work() = %v, want nil", err)
	}
}

func TestDefaultWorkers(t *testing.T) {
	t.Parallel()

	if queue.DefaultWorkers() == nil {
		t.Fatal("DefaultWorkers() returned nil")
	}
}

func TestNewWorkerClient(t *testing.T) {
	t.Parallel()

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	client, err := queue.NewWorkerClient(db, queue.DefaultWorkers(), 4, logger())
	if err != nil {
		t.Fatalf("NewWorkerClient: %v", err)
	}

	if client == nil {
		t.Error("client is nil")
	}
}

type fakeLifecycle struct {
	startErr error
	stopErr  error
	started  bool
	stopped  bool
}

func (f *fakeLifecycle) Start(context.Context) error {
	f.started = true

	return f.startErr
}

func (f *fakeLifecycle) Stop(context.Context) error {
	f.stopped = true

	return f.stopErr
}

func TestService_Lifecycle(t *testing.T) {
	t.Parallel()

	lc := &fakeLifecycle{}
	svc := queue.NewService("worker", lc)

	if svc.Name() != "worker" {
		t.Errorf("Name() = %q, want worker", svc.Name())
	}

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !lc.started {
		t.Error("underlying client not started")
	}

	if err := svc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if !lc.stopped {
		t.Error("underlying client not stopped")
	}
}

func TestService_PropagatesErrors(t *testing.T) {
	t.Parallel()

	startErr := errors.New("cannot start")
	stopErr := errors.New("cannot stop")
	svc := queue.NewService("worker", &fakeLifecycle{startErr: startErr, stopErr: stopErr})

	if err := svc.Start(context.Background()); !errors.Is(err, startErr) {
		t.Errorf("Start err = %v, want %v", err, startErr)
	}

	if err := svc.Stop(context.Background()); !errors.Is(err, stopErr) {
		t.Errorf("Stop err = %v, want %v", err, stopErr)
	}
}
