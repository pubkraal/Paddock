// Package config loads and validates the typed, per-deployable configuration
// from the process environment. Each deployable (web, worker, ftp-gateway) has
// its own Load function that fails fast, listing every missing required var at
// once rather than one at a time. Loaders take a getenv func so they are pure
// and testable without mutating the global environment.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrMissingConfig is returned (wrapped) when one or more required environment
// variables are absent. The wrapped message names every missing key.
var ErrMissingConfig = errors.New("missing required configuration")

// Postgres is the application database connection (the non-superuser, RLS-bound
// role per ADR-0008). The migration URL is a tooling concern, not a runtime one.
type Postgres struct {
	URL string
}

// Redis is the ephemeral store: sessions, magic-link tokens, rate limits.
type Redis struct {
	Addr     string
	Password string
	DB       int
}

// ObjectStore is the S3-compatible, EU-region object storage (MinIO in dev).
type ObjectStore struct {
	Endpoint     string
	Region       string
	AccessKey    string
	SecretKey    string
	Bucket       string
	UsePathStyle bool
}

// HTTP is the serving config for cmd/web.
type HTTP struct {
	Addr            string
	ShutdownTimeout time.Duration
}

// Auth is the passwordless magic-link session config (ADR-0013). BaseURL is the
// externally-reachable origin used to build magic-link URLs.
type Auth struct {
	BaseURL      string
	SessionTTL   time.Duration
	MagicLinkTTL time.Duration
	CookieSecure bool
}

// Mailer is the SMTP transport config (ADR-0007). SMTPAddr defaults to the dev
// Mailpit sink; From is the envelope/From address on outbound mail.
type Mailer struct {
	SMTPAddr string
	From     string
}

// Web is the full configuration for cmd/web.
type Web struct {
	HTTP        HTTP
	Postgres    Postgres
	Redis       Redis
	ObjectStore ObjectStore
	Auth        Auth
	Mailer      Mailer
}

// Worker is the full configuration for cmd/worker. It carries Redis, Mailer and
// Auth because the worker now sends accreditation-invite magic links (ADR-0016):
// it issues consumer-grant tokens (Redis), builds links from Auth.BaseURL, and
// sends them via the Mailer.
type Worker struct {
	Postgres        Postgres
	Redis           Redis
	ObjectStore     ObjectStore
	Auth            Auth
	Mailer          Mailer
	Concurrency     int
	ShutdownTimeout time.Duration
}

// FTPGateway is the full configuration for cmd/ftp-gateway.
type FTPGateway struct {
	Addr            string
	Postgres        Postgres
	ObjectStore     ObjectStore
	ShutdownTimeout time.Duration
}

// LoadWeb assembles and validates the cmd/web configuration.
func LoadWeb(getenv func(string) string) (Web, error) {
	r := newReader(getenv)
	cfg := Web{
		HTTP: HTTP{
			Addr:            r.optional("PADDOCK_HTTP_ADDR", ":8080"),
			ShutdownTimeout: r.duration("PADDOCK_SHUTDOWN_TIMEOUT", 15*time.Second),
		},
		Postgres:    r.postgres(),
		Redis:       r.redis(),
		ObjectStore: r.objectStore(),
		Auth:        r.auth(),
		Mailer:      r.mailer(),
	}

	if err := r.err(); err != nil {
		return Web{}, err
	}

	return cfg, nil
}

// LoadWorker assembles and validates the cmd/worker configuration.
func LoadWorker(getenv func(string) string) (Worker, error) {
	r := newReader(getenv)
	cfg := Worker{
		Postgres:        r.postgres(),
		Redis:           r.redis(),
		ObjectStore:     r.objectStore(),
		Auth:            r.auth(),
		Mailer:          r.mailer(),
		Concurrency:     r.positiveInt("WORKER_CONCURRENCY", 8),
		ShutdownTimeout: r.duration("PADDOCK_SHUTDOWN_TIMEOUT", 30*time.Second),
	}

	if err := r.err(); err != nil {
		return Worker{}, err
	}

	return cfg, nil
}

// LoadFTPGateway assembles and validates the cmd/ftp-gateway configuration.
func LoadFTPGateway(getenv func(string) string) (FTPGateway, error) {
	r := newReader(getenv)
	cfg := FTPGateway{
		Addr:            r.optional("FTP_ADDR", ":2022"),
		Postgres:        r.postgres(),
		ObjectStore:     r.objectStore(),
		ShutdownTimeout: r.duration("PADDOCK_SHUTDOWN_TIMEOUT", 15*time.Second),
	}

	if err := r.err(); err != nil {
		return FTPGateway{}, err
	}

	return cfg, nil
}

type reader struct {
	getenv  func(string) string
	missing []string
	invalid []string
}

func newReader(getenv func(string) string) *reader {
	return &reader{getenv: getenv}
}

func (r *reader) required(key string) string {
	v := r.getenv(key)
	if v == "" {
		r.missing = append(r.missing, key)
	}

	return v
}

func (r *reader) optional(key, def string) string {
	if v := r.getenv(key); v != "" {
		return v
	}

	return def
}

func (r *reader) duration(key string, def time.Duration) time.Duration {
	v := r.getenv(key)
	if v == "" {
		return def
	}

	d, err := time.ParseDuration(v)
	if err != nil {
		r.invalid = append(r.invalid, fmt.Sprintf("%s (%q is not a duration)", key, v))

		return def
	}

	return d
}

func (r *reader) positiveInt(key string, def int) int {
	v := r.getenv(key)
	if v == "" {
		return def
	}

	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		r.invalid = append(r.invalid, fmt.Sprintf("%s (%q is not a positive integer)", key, v))

		return def
	}

	return n
}

func (r *reader) boolean(key string, def bool) bool {
	v := r.getenv(key)
	if v == "" {
		return def
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		r.invalid = append(r.invalid, fmt.Sprintf("%s (%q is not a boolean)", key, v))

		return def
	}

	return b
}

func (r *reader) postgres() Postgres {
	return Postgres{URL: r.required("DATABASE_URL")}
}

func (r *reader) redis() Redis {
	return Redis{
		Addr:     r.optional("REDIS_ADDR", "localhost:6379"),
		Password: r.optional("REDIS_PASSWORD", ""),
		DB:       r.positiveOrZeroInt("REDIS_DB", 0),
	}
}

func (r *reader) positiveOrZeroInt(key string, def int) int {
	v := r.getenv(key)
	if v == "" {
		return def
	}

	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		r.invalid = append(r.invalid, fmt.Sprintf("%s (%q is not a non-negative integer)", key, v))

		return def
	}

	return n
}

func (r *reader) auth() Auth {
	return Auth{
		BaseURL:      r.required("PADDOCK_BASE_URL"),
		SessionTTL:   r.duration("PADDOCK_SESSION_TTL", 12*time.Hour),
		MagicLinkTTL: r.duration("PADDOCK_MAGICLINK_TTL", 15*time.Minute),
		CookieSecure: r.boolean("PADDOCK_COOKIE_SECURE", true),
	}
}

func (r *reader) mailer() Mailer {
	return Mailer{
		SMTPAddr: r.optional("PADDOCK_SMTP_ADDR", "localhost:1025"),
		From:     r.required("PADDOCK_MAIL_FROM"),
	}
}

func (r *reader) objectStore() ObjectStore {
	return ObjectStore{
		Endpoint:     r.required("S3_ENDPOINT"),
		Region:       r.optional("S3_REGION", "eu-central-1"),
		AccessKey:    r.required("S3_ACCESS_KEY_ID"),
		SecretKey:    r.required("S3_SECRET_ACCESS_KEY"),
		Bucket:       r.required("S3_BUCKET"),
		UsePathStyle: r.boolean("S3_USE_PATH_STYLE", true),
	}
}

func (r *reader) err() error {
	if len(r.missing) == 0 && len(r.invalid) == 0 {
		return nil
	}

	var b strings.Builder

	if len(r.missing) > 0 {
		fmt.Fprintf(&b, "missing: %s", strings.Join(r.missing, ", "))
	}

	if len(r.invalid) > 0 {
		if b.Len() > 0 {
			b.WriteString("; ")
		}

		fmt.Fprintf(&b, "invalid: %s", strings.Join(r.invalid, ", "))
	}

	return fmt.Errorf("%w: %s", ErrMissingConfig, b.String())
}
