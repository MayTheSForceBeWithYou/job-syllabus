// Package api implements the read-only REST surface from docs/design.md
// §7 for Phase 3 ("Deploy API"): GET /v1/skills, /v1/skills/{id},
// /v1/skills/{id}/postings, /v1/postings, /v1/postings/{id},
// /v1/companies, /v1/stats/overview, /healthz, /readyz. Write endpoints
// (submit/reviews/exports) and the Cognito JWT authorizer are later
// phases — Phase 3's own DoD is explicit that this is read-only, "no auth
// yet (locked to your IP)" at the network layer (API Gateway + WAF).
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/connectors"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/store"
)

// Server holds the API's dependencies. Skills/Companies are the
// data/*.yaml registries loaded once at startup (same source of truth
// cmd/ingest uses) — precomputed into lookup maps here since every
// request that touches them would otherwise rebuild the same map.
type Server struct {
	Store     *store.Store
	Skills    []model.Skill
	Companies []connectors.CompanyConfig

	skillsByID map[string]model.Skill
	tierBySlug map[string]string
}

// New builds a Server, precomputing its lookup maps.
func New(s *store.Store, skills []model.Skill, companies []connectors.CompanyConfig) *Server {
	skillsByID := make(map[string]model.Skill, len(skills))
	for _, sk := range skills {
		skillsByID[sk.ID] = sk
	}
	tierBySlug := make(map[string]string, len(companies))
	for _, c := range companies {
		tierBySlug[c.Slug] = c.Tier
	}
	return &Server{
		Store:      s,
		Skills:     skills,
		Companies:  companies,
		skillsByID: skillsByID,
		tierBySlug: tierBySlug,
	}
}

// Router builds the full HTTP handler tree.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(recoverer)
	r.Use(requestID)
	r.Use(requestLogger)

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, r, http.StatusNotFound, "not found", "no route matches "+r.URL.Path)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, r, http.StatusMethodNotAllowed, "method not allowed", r.Method+" is not supported for "+r.URL.Path)
	})

	// Unauthenticated, no VPC egress — docs/design.md §7. These are ALB
	// target-group health checks, not application endpoints.
	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleReadyz)

	r.Route("/v1", func(r chi.Router) {
		r.Get("/skills", s.handleListSkills)
		r.Get("/skills/{id}", s.handleGetSkill)
		r.Get("/skills/{id}/postings", s.handleGetSkillPostings)
		r.Get("/postings", s.handleListPostings)
		r.Get("/postings/{id}", s.handleGetPosting)
		r.Get("/companies", s.handleListCompanies)
		r.Get("/stats/overview", s.handleStatsOverview)
	})

	return r
}
