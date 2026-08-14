package api

import "net/http"

// handleHealthz is a pure liveness check — no dependency calls, so it
// stays fast and reliable even if DynamoDB is having a bad day.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

// handleReadyz checks the one real dependency (DynamoDB) so the ALB stops
// routing traffic to a task that's up but can't actually serve anything.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Ping(r.Context()); err != nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "not ready", err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ready"})
}
