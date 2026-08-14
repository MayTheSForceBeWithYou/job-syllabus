package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDGeneratesAndEchoesHeader(t *testing.T) {
	var seenInContext string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenInContext = requestIDFromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	requestID(next).ServeHTTP(rec, req)

	header := rec.Header().Get("X-Request-ID")
	if header == "" {
		t.Fatal("X-Request-ID header not set")
	}
	if seenInContext != header {
		t.Errorf("context request ID %q != response header %q", seenInContext, header)
	}
}

func TestRequestIDReusesInboundHeader(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "caller-supplied-id")
	rec := httptest.NewRecorder()
	requestID(next).ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "caller-supplied-id" {
		t.Errorf("X-Request-ID = %q, want caller-supplied-id to be preserved", got)
	}
}

func TestRecovererCatchesPanic(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	rec := httptest.NewRecorder()

	// Must not panic out of the test itself.
	recoverer(panicking).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
