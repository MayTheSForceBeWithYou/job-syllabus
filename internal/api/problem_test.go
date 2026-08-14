package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteProblem(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/skills/bogus", nil)
	rec := httptest.NewRecorder()

	writeProblem(rec, req, http.StatusNotFound, "skill not found", "no skill with id bogus")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if p.Status != http.StatusNotFound || p.Title != "skill not found" || p.Instance != "/v1/skills/bogus" {
		t.Errorf("problem = %+v, unexpected fields", p)
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, map[string]string{"status": "ok"})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if got := rec.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Errorf("body = %q", got)
	}
}
