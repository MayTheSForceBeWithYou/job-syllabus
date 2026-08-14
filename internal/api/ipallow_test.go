package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPAllowlistDisabledWhenEmpty(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	rec := httptest.NewRecorder()
	ipAllowlist("")(next).ServeHTTP(rec, req)

	if !called {
		t.Error("next handler was not called when allowlist is disabled")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestIPAllowlistAllowsMatchingIP(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	req.Header.Set("X-Forwarded-For", "107.140.215.30")
	rec := httptest.NewRecorder()
	ipAllowlist("107.140.215.30/32")(next).ServeHTTP(rec, req)

	if !called {
		t.Error("next handler was not called for an allowed IP")
	}
}

func TestIPAllowlistBlocksNonMatchingIP(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	req.Header.Set("X-Forwarded-For", "8.8.8.8")
	rec := httptest.NewRecorder()
	ipAllowlist("107.140.215.30/32")(next).ServeHTTP(rec, req)

	if called {
		t.Error("next handler was called for a disallowed IP")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestIPAllowlistUsesFirstForwardedIP(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	// The allowed client IP first, followed by an intermediate hop's IP —
	// only the first entry should be checked.
	req.Header.Set("X-Forwarded-For", "107.140.215.30, 10.20.0.55")
	rec := httptest.NewRecorder()
	ipAllowlist("107.140.215.30/32")(next).ServeHTTP(rec, req)

	if !called {
		t.Error("next handler was not called when the first X-Forwarded-For entry is allowed")
	}
}

func TestIPAllowlistBlocksMissingHeader(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	rec := httptest.NewRecorder()
	ipAllowlist("107.140.215.30/32")(next).ServeHTTP(rec, req)

	if called {
		t.Error("next handler was called with no X-Forwarded-For header at all")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestIPAllowlistPanicsOnInvalidCIDR(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("ipAllowlist did not panic on an invalid CIDR")
		}
	}()
	ipAllowlist("not-a-cidr")
}
