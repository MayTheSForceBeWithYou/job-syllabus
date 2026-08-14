package api

import (
	"log/slog"
	"net"
	"net/http"
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
// where there's no API Gateway in front to set X-Real-Ip at all).
//
// Trust note: this reads X-Real-Ip, not the conventional X-Forwarded-For.
// A real request from the operator's own allowed IP proved
// X-Forwarded-For does NOT carry the true external client IP through this
// service's API Gateway (HTTP API) -> VPC Link -> internal ALB path —
// what actually arrives is the VPC Link's own internal VPC address (the
// hop between the VPC Link and the ALB), not anything traceable back to
// the original caller. X-Real-Ip is populated instead by an explicit
// overwrite:header parameter mapping on the API Gateway integration
// (modules/api-gateway/main.tf), set to $context.identity.sourceIp —
// API Gateway's own authoritative view of the caller, which it doesn't
// otherwise propagate for private integrations. overwrite:, not append:,
// on that mapping means a client-supplied X-Real-Ip can't survive to
// spoof this check. This is explicitly a lightweight placeholder, not the
// final security posture: Phase 6 replaces it with a real Cognito JWT
// authorizer.
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

			realIP := r.Header.Get("X-Real-Ip")
			ip := net.ParseIP(realIP)
			if ip == nil || !allowed.Contains(ip) {
				slog.Warn("api: ip allowlist rejected request",
					"path", r.URL.Path, "xRealIp", realIP, "remoteAddr", r.RemoteAddr)
				writeProblem(w, r, http.StatusForbidden, "forbidden", "")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
