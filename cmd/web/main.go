// Command web serves the Paddock admin UI, branded portals, and delivery
// endpoints (ADR-0001). Phase 0 boots the walking skeleton: health endpoints,
// the static asset server, and the throwaway /_styleguide route.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/julienschmidt/httprouter"
	"github.com/pubkraal/paddock/internal/platform/config"
	"github.com/pubkraal/paddock/internal/platform/httpx"
	"github.com/pubkraal/paddock/internal/platform/objectstore"
	"github.com/pubkraal/paddock/internal/platform/postgres"
	"github.com/pubkraal/paddock/internal/platform/redis"
	"github.com/pubkraal/paddock/internal/platform/runtime"
	"github.com/pubkraal/paddock/web"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("web exited with error", slog.Any("err", err))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.LoadWeb(os.Getenv)
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

	store := objectstore.Open(cfg.ObjectStore)

	renderer, err := web.NewRenderer()
	if err != nil {
		return err
	}

	router := httprouter.New()
	router.HandlerFunc("GET", "/healthz", httpx.Healthz())
	router.HandlerFunc("GET", "/readyz", httpx.Readyz(map[string]httpx.Check{
		"postgres":    pool.Ping,
		"redis":       rdb.Ping,
		"objectstore": store.Ping,
	}))
	router.HandlerFunc("GET", "/_styleguide", renderer.PageHandler("styleguide"))
	router.Handler("GET", "/static/*filepath", web.StaticHandler())

	handler := httpx.Chain(router, httpx.RequestID, httpx.Recovery(logger), httpx.Logging(logger))
	server := httpx.NewServer("web", cfg.HTTP.Addr, handler, logger)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	return runtime.Run(ctx, logger, cfg.HTTP.ShutdownTimeout, sig, server)
}
