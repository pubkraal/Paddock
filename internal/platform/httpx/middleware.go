// Package httpx provides the shared HTTP plumbing for cmd/web: a small
// middleware set (request-id, panic recovery, structured request logging), a
// chain helper, and the liveness/readiness handlers. It deliberately holds no
// domain logic — handlers are defined in the packages that own their behaviour.
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

type contextKey int

const requestIDKey contextKey = iota

const requestIDHeader = "X-Request-Id"

// Chain wraps h with the given middleware so that the first listed runs
// outermost (first on the way in, last on the way out).
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}

	return h
}

// RequestID ensures every request carries a stable identifier: it honours an
// inbound X-Request-Id, otherwise generates one, stores it on the context, and
// echoes it on the response.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}

		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext returns the request id stored on ctx, or "" if absent.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)

	return id
}

func newRequestID() string {
	var b [16]byte

	_, _ = rand.Read(b[:])

	return hex.EncodeToString(b[:])
}

// Recovery converts a panic in a downstream handler into a 500 response and
// logs it, so a single bad handler never takes the server down.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.ErrorContext(r.Context(), "handler panic",
						slog.Any("panic", rec),
						slog.String("request_id", RequestIDFromContext(r.Context())),
					)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// Logging emits one structured record per request with method, path, status,
// duration, and request id.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			logger.InfoContext(r.Context(), "request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", RequestIDFromContext(r.Context())),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}

	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wroteHeader = true

	return s.ResponseWriter.Write(b)
}
