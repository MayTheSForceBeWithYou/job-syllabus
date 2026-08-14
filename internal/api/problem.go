package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Problem is an RFC 7807 problem+json body (docs/design.md §7's error
// convention).
type Problem struct {
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(Problem{
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: r.URL.Path,
	}); err != nil {
		slog.Error("api: failed to encode problem response", "error", err.Error())
	}
}

// writeJSON always responds 200 — every handler in this package is a GET
// that either succeeds or returns a Problem via writeProblem instead.
// Add a status param back if a future write endpoint needs 201/202.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("api: failed to encode json response", "error", err.Error())
	}
}
