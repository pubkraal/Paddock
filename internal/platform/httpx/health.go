package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
)

// Check is a readiness probe for one dependency. It returns nil when the
// dependency is reachable and usable, or an error describing why it is not.
type Check func(ctx context.Context) error

// Healthz is the liveness probe: it reports that the process is up and serving.
// It performs no dependency checks — a live-but-not-ready process still answers
// 200 here so orchestrators do not restart it while it warms up.
func Healthz() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// Readyz is the readiness probe: it runs every check and reports 200 only when
// all pass, otherwise 503 with the names and reasons of those that failed.
func Readyz(checks map[string]Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		failed := make(map[string]string)

		for _, name := range sortedKeys(checks) {
			if err := checks[name](r.Context()); err != nil {
				failed[name] = err.Error()
			}
		}

		if len(failed) > 0 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "unavailable",
				"failed": failed,
			})

			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func sortedKeys(checks map[string]Check) []string {
	names := make([]string, 0, len(checks))
	for name := range checks {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
