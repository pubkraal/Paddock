package config_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pubkraal/paddock/internal/platform/config"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func completeEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":         "postgres://paddock_app:paddock_app@localhost:5432/paddock?sslmode=disable",
		"S3_ENDPOINT":          "http://localhost:9000",
		"S3_ACCESS_KEY_ID":     "minioadmin",
		"S3_SECRET_ACCESS_KEY": "minioadmin",
		"S3_BUCKET":            "paddock-eu",
		"PADDOCK_BASE_URL":     "https://paddock.test",
		"PADDOCK_MAIL_FROM":    "no-reply@paddock.test",
	}
}

func TestLoadWeb_Defaults(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadWeb(envFrom(completeEnv()))
	if err != nil {
		t.Fatalf("LoadWeb: %v", err)
	}

	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("HTTP.Addr = %q, want default %q", cfg.HTTP.Addr, ":8080")
	}

	if cfg.HTTP.ShutdownTimeout != 15*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 15s", cfg.HTTP.ShutdownTimeout)
	}

	if cfg.Redis.Addr != "localhost:6379" {
		t.Errorf("Redis.Addr = %q, want default", cfg.Redis.Addr)
	}

	if cfg.ObjectStore.Region != "eu-central-1" {
		t.Errorf("ObjectStore.Region = %q, want EU default", cfg.ObjectStore.Region)
	}

	if !cfg.ObjectStore.UsePathStyle {
		t.Error("ObjectStore.UsePathStyle = false, want true by default (MinIO)")
	}

	if cfg.Postgres.URL != completeEnv()["DATABASE_URL"] {
		t.Errorf("Postgres.URL = %q, want the DATABASE_URL", cfg.Postgres.URL)
	}
}

func TestLoadWeb_AuthAndMailerDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadWeb(envFrom(completeEnv()))
	if err != nil {
		t.Fatalf("LoadWeb: %v", err)
	}

	if cfg.Auth.BaseURL != "https://paddock.test" {
		t.Errorf("Auth.BaseURL = %q, want the PADDOCK_BASE_URL", cfg.Auth.BaseURL)
	}

	if cfg.Auth.SessionTTL != 12*time.Hour {
		t.Errorf("Auth.SessionTTL = %v, want default 12h", cfg.Auth.SessionTTL)
	}

	if cfg.Auth.MagicLinkTTL != 15*time.Minute {
		t.Errorf("Auth.MagicLinkTTL = %v, want default 15m", cfg.Auth.MagicLinkTTL)
	}

	if !cfg.Auth.CookieSecure {
		t.Error("Auth.CookieSecure = false, want secure-by-default")
	}

	if cfg.Mailer.SMTPAddr != "localhost:1025" {
		t.Errorf("Mailer.SMTPAddr = %q, want default Mailpit addr", cfg.Mailer.SMTPAddr)
	}

	if cfg.Mailer.From != "no-reply@paddock.test" {
		t.Errorf("Mailer.From = %q, want the PADDOCK_MAIL_FROM", cfg.Mailer.From)
	}
}

func TestLoadWeb_AuthAndMailerOverrides(t *testing.T) {
	t.Parallel()

	env := completeEnv()
	env["PADDOCK_SESSION_TTL"] = "4h"
	env["PADDOCK_MAGICLINK_TTL"] = "5m"
	env["PADDOCK_COOKIE_SECURE"] = "false"
	env["PADDOCK_SMTP_ADDR"] = "mail:2525"

	cfg, err := config.LoadWeb(envFrom(env))
	if err != nil {
		t.Fatalf("LoadWeb: %v", err)
	}

	if cfg.Auth.SessionTTL != 4*time.Hour {
		t.Errorf("Auth.SessionTTL = %v, want 4h override", cfg.Auth.SessionTTL)
	}

	if cfg.Auth.MagicLinkTTL != 5*time.Minute {
		t.Errorf("Auth.MagicLinkTTL = %v, want 5m override", cfg.Auth.MagicLinkTTL)
	}

	if cfg.Auth.CookieSecure {
		t.Error("Auth.CookieSecure = true, want override to false")
	}

	if cfg.Mailer.SMTPAddr != "mail:2525" {
		t.Errorf("Mailer.SMTPAddr = %q, want override", cfg.Mailer.SMTPAddr)
	}
}

func TestLoadWeb_MissingAuthMailerRequired(t *testing.T) {
	t.Parallel()

	env := completeEnv()
	delete(env, "PADDOCK_BASE_URL")
	delete(env, "PADDOCK_MAIL_FROM")

	_, err := config.LoadWeb(envFrom(env))
	if err == nil {
		t.Fatal("expected error for missing required auth/mailer vars, got nil")
	}

	for _, key := range []string{"PADDOCK_BASE_URL", "PADDOCK_MAIL_FROM"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q should name missing key %q", err.Error(), key)
		}
	}
}

func TestLoadWeb_InvalidSessionTTL(t *testing.T) {
	t.Parallel()

	env := completeEnv()
	env["PADDOCK_SESSION_TTL"] = "forever"

	if _, err := config.LoadWeb(envFrom(env)); err == nil {
		t.Fatal("expected error for invalid session TTL, got nil")
	}
}

func TestLoadWeb_Overrides(t *testing.T) {
	t.Parallel()

	env := completeEnv()
	env["PADDOCK_HTTP_ADDR"] = ":9999"
	env["PADDOCK_SHUTDOWN_TIMEOUT"] = "30s"
	env["REDIS_ADDR"] = "redis:6379"
	env["S3_REGION"] = "eu-west-1"
	env["S3_USE_PATH_STYLE"] = "false"

	cfg, err := config.LoadWeb(envFrom(env))
	if err != nil {
		t.Fatalf("LoadWeb: %v", err)
	}

	if cfg.HTTP.Addr != ":9999" {
		t.Errorf("HTTP.Addr = %q, want override", cfg.HTTP.Addr)
	}

	if cfg.HTTP.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 30s", cfg.HTTP.ShutdownTimeout)
	}

	if cfg.ObjectStore.UsePathStyle {
		t.Error("UsePathStyle = true, want override to false")
	}

	if cfg.ObjectStore.Region != "eu-west-1" {
		t.Errorf("Region = %q, want override", cfg.ObjectStore.Region)
	}
}

func TestLoadWeb_MissingRequired(t *testing.T) {
	t.Parallel()

	env := completeEnv()
	delete(env, "DATABASE_URL")
	delete(env, "S3_BUCKET")

	_, err := config.LoadWeb(envFrom(env))
	if err == nil {
		t.Fatal("expected error for missing required vars, got nil")
	}

	if !errors.Is(err, config.ErrMissingConfig) {
		t.Errorf("error = %v, want ErrMissingConfig", err)
	}

	for _, key := range []string{"DATABASE_URL", "S3_BUCKET"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q should name missing key %q", err.Error(), key)
		}
	}
}

func TestLoadWeb_InvalidDuration(t *testing.T) {
	t.Parallel()

	env := completeEnv()
	env["PADDOCK_SHUTDOWN_TIMEOUT"] = "not-a-duration"

	_, err := config.LoadWeb(envFrom(env))
	if err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}

func TestLoadWeb_InvalidBoolean(t *testing.T) {
	t.Parallel()

	env := completeEnv()
	env["S3_USE_PATH_STYLE"] = "maybe"

	_, err := config.LoadWeb(envFrom(env))
	if err == nil {
		t.Fatal("expected error for invalid boolean, got nil")
	}
}

func TestLoadWeb_RedisDB(t *testing.T) {
	t.Parallel()

	env := completeEnv()
	env["REDIS_DB"] = "3"

	cfg, err := config.LoadWeb(envFrom(env))
	if err != nil {
		t.Fatalf("LoadWeb: %v", err)
	}

	if cfg.Redis.DB != 3 {
		t.Errorf("Redis.DB = %d, want 3", cfg.Redis.DB)
	}

	env["REDIS_DB"] = "-1"

	if _, err := config.LoadWeb(envFrom(env)); err == nil {
		t.Fatal("expected error for negative REDIS_DB, got nil")
	}
}

func TestLoadWeb_MissingAndInvalid(t *testing.T) {
	t.Parallel()

	env := completeEnv()
	delete(env, "DATABASE_URL")
	env["PADDOCK_SHUTDOWN_TIMEOUT"] = "nope"

	_, err := config.LoadWeb(envFrom(env))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "missing") || !strings.Contains(msg, "invalid") {
		t.Errorf("error %q should report both missing and invalid", msg)
	}
}

func TestLoadWorker_ConcurrencyDefaultAndOverride(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadWorker(envFrom(completeEnv()))
	if err != nil {
		t.Fatalf("LoadWorker: %v", err)
	}

	if cfg.Concurrency != 8 {
		t.Errorf("Concurrency = %d, want default 8", cfg.Concurrency)
	}

	env := completeEnv()
	env["WORKER_CONCURRENCY"] = "16"

	cfg, err = config.LoadWorker(envFrom(env))
	if err != nil {
		t.Fatalf("LoadWorker override: %v", err)
	}

	if cfg.Concurrency != 16 {
		t.Errorf("Concurrency = %d, want 16", cfg.Concurrency)
	}
}

func TestLoadWorker_CarriesRedisMailerAuth(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadWorker(envFrom(completeEnv()))
	if err != nil {
		t.Fatalf("LoadWorker: %v", err)
	}

	if cfg.Auth.BaseURL == "" {
		t.Error("Worker.Auth.BaseURL is empty, want the PADDOCK_BASE_URL")
	}

	if cfg.Mailer.From == "" {
		t.Error("Worker.Mailer.From is empty, want the PADDOCK_MAIL_FROM")
	}

	if cfg.Redis.Addr == "" {
		t.Error("Worker.Redis.Addr is empty, want the configured Redis address")
	}
}

func TestLoadWorker_MissingMailAndAuthKeys(t *testing.T) {
	t.Parallel()

	env := completeEnv()
	delete(env, "PADDOCK_BASE_URL")
	delete(env, "PADDOCK_MAIL_FROM")

	_, err := config.LoadWorker(envFrom(env))
	if err == nil {
		t.Fatal("expected error when the worker's mail/auth keys are missing")
	}

	for _, key := range []string{"PADDOCK_BASE_URL", "PADDOCK_MAIL_FROM"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not mention %s", err, key)
		}
	}
}

func TestLoadWorker_InvalidConcurrency(t *testing.T) {
	t.Parallel()

	env := completeEnv()
	env["WORKER_CONCURRENCY"] = "-3"

	_, err := config.LoadWorker(envFrom(env))
	if err == nil {
		t.Fatal("expected error for non-positive concurrency, got nil")
	}
}

func TestLoadFTPGateway_Defaults(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadFTPGateway(envFrom(completeEnv()))
	if err != nil {
		t.Fatalf("LoadFTPGateway: %v", err)
	}

	if cfg.Addr != ":2022" {
		t.Errorf("Addr = %q, want default :2022", cfg.Addr)
	}

	if cfg.Postgres.URL == "" {
		t.Error("Postgres.URL empty, want populated")
	}
}

func TestLoadFTPGateway_MissingRequired(t *testing.T) {
	t.Parallel()

	env := completeEnv()
	delete(env, "S3_ENDPOINT")

	_, err := config.LoadFTPGateway(envFrom(env))
	if err == nil {
		t.Fatal("expected error for missing S3_ENDPOINT, got nil")
	}
}
