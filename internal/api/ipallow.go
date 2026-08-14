package api

import (
	"net"
	"net/http"
	"strings"
)

// ipAllowlist is Phase 3's whole IP-restriction story (docs/design.md
// §13 DoD: "no auth yet, locked to your IP"). It lives here, not at the
// network edge, because AWS WAFv2's AssociateWebACL does not support
// Amazon API Gateway HTTP APIs (protocol_type=HTTP) as a resource type at
// all — confirmed against both a real failing apply and AWS's own
// AssociateWebACL API reference, which lists REST APIs but not HTTP
// APIs. A REST API (v1) would get WAF/resource-policy support back, but
// would also deviate from docs/design.md §3's stated HTTP-API-with-a-
// JWT-authorizer design for Phase 6 — so the fix is here instead.
//
// allowedCIDR empty disables the check entirely (local `go run` testing,
// where there's no API Gateway in front to set X-Forwarded-For at all).
//
// Trust note: this trusts the FIRST entry of X-Forwarded-For as the real
// client IP. That's safe specifically because API Gateway is the one
// setting it here (a managed reverse proxy overwriting/prepending the
// header from the actual TCP connection, the same convention ALB and
// CloudFront both follow) — the only way to reach this service at all is
// through API Gateway -> VPC Link -> the internal ALB, so there's no path
// for an untrusted party to inject their own X-Forwarded-For directly.
// This is explicitly a lightweight placeholder, not the final security
// posture: Phase 6 replaces it with a real Cognito JWT authorizer.
func ipAllowlist(allowedCIDR string) func(http.Handler) http.Handler {
	var allowed *net.IPNet
	if allowedCIDR != "" {
		_, ipNet, err := net.ParseCIDR(allowedCIDR)
		if err != nil {
			// Fail closed at startup, not per-request — a malformed CIDR
			// is a deploy-time config error, not a runtime condition to
			// silently ignore.
			panic("api: invalid ALLOWED_CIDR " + allowedCIDR + ": " + err.Error())
		}
		allowed = ipNet
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if allowed == nil {
				next.ServeHTTP(w, r)
				return
			}

			clientIP := firstForwardedIP(r.Header.Get("X-Forwarded-For"))
			ip := net.ParseIP(clientIP)
			if ip == nil || !allowed.Contains(ip) {
				writeProblem(w, r, http.StatusForbidden, "forbidden", "")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func firstForwardedIP(xff string) string {
	first, _, _ := strings.Cut(xff, ",")
	return strings.TrimSpace(first)
}
