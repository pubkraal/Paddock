package runtime_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pubkraal/paddock/internal/platform/runtime"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeService struct {
	name       string
	startErr   error
	stopErr    error
	mu         sync.Mutex
	started    bool
	stopped    bool
	stopOrder  *[]string
	startOrder *[]string
}

func (f *fakeService) Name() string { return f.name }

func (f *fakeService) Start(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.startOrder != nil {
		*f.startOrder = append(*f.startOrder, f.name)
	}

	if f.startErr != nil {
		return f.startErr
	}

	f.started = true

	return nil
}

func (f *fakeService) Stop(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.stopOrder != nil {
		*f.stopOrder = append(*f.stopOrder, f.name)
	}

	f.stopped = true

	return f.stopErr
}

func (f *fakeService) wasStopped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.stopped
}

func TestRun_StartsThenShutsDownOnSignal(t *testing.T) {
	t.Parallel()

	var startOrder, stopOrder []string
	a := &fakeService{name: "a", startOrder: &startOrder, stopOrder: &stopOrder}
	b := &fakeService{name: "b", startOrder: &startOrder, stopOrder: &stopOrder}

	sig := make(chan os.Signal, 1)

	done := make(chan error, 1)
	go func() {
		done <- runtime.Run(context.Background(), discardLogger(), time.Second, sig, a, b)
	}()

	sig <- os.Interrupt

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after signal")
	}

	if len(startOrder) != 2 || startOrder[0] != "a" || startOrder[1] != "b" {
		t.Errorf("start order = %v, want [a b]", startOrder)
	}

	if len(stopOrder) != 2 || stopOrder[0] != "b" || stopOrder[1] != "a" {
		t.Errorf("stop order = %v, want reverse [b a]", stopOrder)
	}
}

func TestRun_ShutsDownOnContextCancel(t *testing.T) {
	t.Parallel()

	a := &fakeService{name: "a"}

	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)

	done := make(chan error, 1)
	go func() {
		done <- runtime.Run(ctx, discardLogger(), time.Second, sig, a)
	}()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}

	if !a.wasStopped() {
		t.Error("service was not stopped on context cancel")
	}
}

func TestRun_StartFailureStopsAlreadyStarted(t *testing.T) {
	t.Parallel()

	startFail := errors.New("port in use")
	a := &fakeService{name: "a"}
	b := &fakeService{name: "b", startErr: startFail}
	c := &fakeService{name: "c"}

	err := runtime.Run(context.Background(), discardLogger(), time.Second, make(chan os.Signal, 1), a, b, c)
	if !errors.Is(err, startFail) {
		t.Fatalf("err = %v, want %v", err, startFail)
	}

	if !a.wasStopped() {
		t.Error("already-started service a should have been stopped")
	}

	if c.wasStopped() {
		t.Error("service c after the failure should never have been started or stopped")
	}
}

func TestRun_AggregatesShutdownErrors(t *testing.T) {
	t.Parallel()

	stopFail := errors.New("drain timeout")
	a := &fakeService{name: "a", stopErr: stopFail}

	sig := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() {
		done <- runtime.Run(context.Background(), discardLogger(), time.Second, sig, a)
	}()

	sig <- os.Interrupt

	select {
	case err := <-done:
		if !errors.Is(err, stopFail) {
			t.Fatalf("err = %v, want %v", err, stopFail)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
}
