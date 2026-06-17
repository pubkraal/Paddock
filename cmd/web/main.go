// Command web serves the Paddock admin UI, branded portals, and delivery
// endpoints (ADR-0001). Phase 1 adds passwordless magic-link access: the login
// and landing screens, the magic-link request/redeem flow, and Redis-backed
// sessions, alongside the Phase 0 health, static, and styleguide routes.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/pubkraal/paddock/internal/catalog"
	"github.com/pubkraal/paddock/internal/identity"
	"github.com/pubkraal/paddock/internal/invite"
	"github.com/pubkraal/paddock/internal/platform/config"
	"github.com/pubkraal/paddock/internal/platform/httpx"
	"github.com/pubkraal/paddock/internal/platform/mailer"
	"github.com/pubkraal/paddock/internal/platform/objectstore"
	"github.com/pubkraal/paddock/internal/platform/postgres"
	"github.com/pubkraal/paddock/internal/platform/queue"
	"github.com/pubkraal/paddock/internal/platform/redis"
	"github.com/pubkraal/paddock/internal/platform/runtime"
	"github.com/pubkraal/paddock/web"
)

// mailSendTimeout bounds how long a magic-link send may hold the request open,
// so a slow SMTP relay cannot widen the anti-enumeration timing window.
const mailSendTimeout = 5 * time.Second

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

	mail := mailer.NewSMTPMailer(cfg.Mailer.SMTPAddr, cfg.Mailer.From)
	tokens := identity.NewTokenStore(rdb, cfg.Auth.MagicLinkTTL)
	sessions := identity.NewSessionStore(rdb, cfg.Auth.SessionTTL)
	repo := identity.NewRepository(pool)
	linkMailer := identity.NewLinkMailer(mail, mailSendTimeout)
	svc := identity.NewService(repo, tokens, sessions, linkMailer, cfg.Auth.BaseURL, time.Now)

	ih := identity.NewHandler(identity.HandlerConfig{
		Service:      svc,
		Users:        repo,
		Renderer:     renderer,
		Logger:       logger,
		CookieSecure: cfg.Auth.CookieSecure,
		SessionTTL:   cfg.Auth.SessionTTL,
	})

	insertClient, err := queue.NewInsertClient(pool.SQL(), logger)
	if err != nil {
		return err
	}

	catalogRepo := catalog.NewRepository(pool)
	catalogSvc := catalog.NewService(catalogRepo, repo, invite.NewEnqueuer(insertClient))
	ch := catalog.NewHandler(catalog.HandlerConfig{Service: catalogSvc, Renderer: renderer, Logger: logger})

	router := httprouter.New()
	router.HandlerFunc("GET", "/healthz", httpx.Healthz())
	router.HandlerFunc("GET", "/readyz", httpx.Readyz(map[string]httpx.Check{
		"postgres":    pool.Ping,
		"redis":       rdb.Ping,
		"objectstore": store.Ping,
	}))
	if cfg.Dev {
		router.HandlerFunc("GET", "/_styleguide", renderer.PageHandler("styleguide"))
	}

	router.Handler("GET", "/static/*filepath", web.StaticHandler())

	router.HandlerFunc("GET", "/login", ih.LoginPage())
	router.HandlerFunc("POST", "/auth/magic", ih.RequestLink())
	router.HandlerFunc("GET", "/auth/redeem", ih.Redeem())
	router.HandlerFunc("POST", "/logout", ih.Logout())

	authenticate := identity.Authenticate(sessions)
	admin := func(h http.Handler) http.Handler { return httpx.Chain(h, authenticate, identity.RequireAdmin) }

	router.Handler("GET", "/admin", admin(ch.Dashboard()))
	router.Handler("GET", "/admin/events/new", admin(ch.NewEvent()))
	router.Handler("POST", "/admin/events", admin(ch.CreateEvent()))
	router.Handler("POST", "/admin/events/:id/entry-list", admin(ch.UploadEntryList()))
	router.Handler("POST", "/admin/events/:id/accreditation", admin(ch.UploadAccreditation()))
	router.Handler("POST", "/admin/events/:id/go-live", admin(ch.GoLive()))
	router.Handler("GET", "/admin/archive", admin(ch.Archive()))
	router.Handler("GET", "/portal", httpx.Chain(ch.Portal(), authenticate))

	handler := httpx.Chain(router,
		httpx.RequestID,
		httpx.SecurityHeaders(cfg.Auth.CookieSecure),
		httpx.Recovery(logger),
		httpx.Logging(logger),
	)
	server := httpx.NewServer("web", cfg.HTTP.Addr, handler, logger)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	return runtime.Run(ctx, logger, cfg.HTTP.ShutdownTimeout, sig, server)
}
