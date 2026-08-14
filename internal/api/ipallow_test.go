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
	req.Header.Set("X-Real-Ip", "107.140.215.30")
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
	req.Header.Set("X-Real-Ip", "8.8.8.8")
	rec := httptest.NewRecorder()
	ipAllowlist("107.140.215.30/32")(next).ServeHTTP(rec, req)

	if called {
		t.Error("next handler was called for a disallowed IP")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestIPAllowlistIgnoresXForwardedFor(t *testing.T) {
	// X-Forwarded-For does not carry the real client IP through this
	// service's API Gateway -> VPC Link -> ALB path at all (it's the VPC
	// Link's own internal hop address instead) — confirmed against a real
	// request from the operator's own allowed IP that was rejected until
	// the allowlist switched to X-Real-Ip. A client setting
	// X-Forwarded-For to an allowed IP must not bypass the check.
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	req.Header.Set("X-Forwarded-For", "107.140.215.30")
	rec := httptest.NewRecorder()
	ipAllowlist("107.140.215.30/32")(next).ServeHTTP(rec, req)

	if called {
		t.Error("next handler was called based on X-Forwarded-For instead of X-Real-Ip")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestIPAllowlistBlocksMissingHeader(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	rec := httptest.NewRecorder()
	ipAllowlist("107.140.215.30/32")(next).ServeHTTP(rec, req)

	if called {
		t.Error("next handler was called with no X-Real-Ip header at all")
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
