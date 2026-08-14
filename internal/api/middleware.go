package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

type ctxKey int

const requestIDKey ctxKey = iota

// requestID stamps every request with an ID (reusing an inbound
// X-Request-ID if the caller already set one — API Gateway can be
// configured to forward one later), echoes it on the response, and makes
// it available to handlers/logging. docs/design.md §7: "X-Request-ID
// echoed and logged."
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read failing means the OS's entropy source is
		// broken — extremely rare, and a missing/duplicate request ID
		// isn't worth failing the request over.
		return "unavailable"
	}
	return hex.EncodeToString(b)
}

func requestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// requestLogger logs one structured line per request via slog, matching
// the connectors/store packages' existing logging convention.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method, "path", r.URL.Path, "status", rec.status,
			"elapsed", time.Since(start).String(), "requestId", requestIDFromContext(r.Context()))
	})
}

// recoverer turns a panicking handler into a 500 problem+json response
// instead of taking down the whole process (or, worse, hanging the
// connection) — this is a public-ish endpoint, so a bad query parameter
// causing a nil-pointer panic must not be able to kill the task.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("api: panic recovered", "path", r.URL.Path, "panic", rec)
				writeProblem(w, r, http.StatusInternalServerError, "internal server error", "")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
