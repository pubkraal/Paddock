// Package runtime orchestrates the lifecycle shared by all three deployables
// (ADR-0001): start each service, wait for a termination signal or a cancelled
// context, then stop the services in reverse order within a bounded timeout.
// This is where the "disposability / graceful shutdown" cross-cutting concern
// (PLAN §3) lives, so each cmd only wires its own services.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// Service is one startable, stoppable component of a deployable — an HTTP
// server, a River client, an FTP listener. Start must not block: it kicks the
// component off and returns, or returns an error if it could not start. Stop
// drains the component within the supplied (timeout-bounded) context.
type Service interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Run starts every service in order, blocks until a signal arrives on signals
// or ctx is cancelled, then stops the started services in reverse order, each
// bounded by shutdownTimeout. If a service fails to start, those already started
// are stopped and the start error is returned. Shutdown errors are joined.
func Run(
	ctx context.Context,
	logger *slog.Logger,
	shutdownTimeout time.Duration,
	signals <-chan os.Signal,
	services ...Service,
) error {
	started := make([]Service, 0, len(services))

	for _, svc := range services {
		if err := svc.Start(ctx); err != nil {
			logger.ErrorContext(ctx, "service failed to start", slog.String("service", svc.Name()), slog.Any("err", err))
			_ = shutdown(logger, shutdownTimeout, started)

			return fmt.Errorf("start %s: %w", svc.Name(), err)
		}

		logger.InfoContext(ctx, "service started", slog.String("service", svc.Name()))
		started = append(started, svc)
	}

	select {
	case sig := <-signals:
		logger.InfoContext(ctx, "shutdown signal received", slog.String("signal", sig.String()))
	case <-ctx.Done():
		logger.InfoContext(ctx, "context cancelled, shutting down")
	}

	return shutdown(logger, shutdownTimeout, started)
}

func shutdown(logger *slog.Logger, timeout time.Duration, started []Service) error {
	var errs []error

	for i := len(started) - 1; i >= 0; i-- {
		svc := started[i]

		ctx, cancel := context.WithTimeout(context.Background(), timeout)

		if err := svc.Stop(ctx); err != nil {
			logger.ErrorContext(ctx, "service failed to stop", slog.String("service", svc.Name()), slog.Any("err", err))
			errs = append(errs, fmt.Errorf("stop %s: %w", svc.Name(), err))
		} else {
			logger.InfoContext(ctx, "service stopped", slog.String("service", svc.Name()))
		}

		cancel()
	}

	return errors.Join(errs...)
}
