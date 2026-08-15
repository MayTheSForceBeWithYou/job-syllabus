// Package api implements the read-only REST surface from docs/design.md
// §7 for Phase 3 ("Deploy API"): GET /v1/skills, /v1/skills/{id},
// /v1/skills/{id}/postings, /v1/postings, /v1/postings/{id},
// /v1/companies, /v1/stats/overview, /healthz, /readyz. Write endpoints
// (submit/reviews/exports) and the Cognito JWT authorizer are later
// phases — Phase 3's own DoD is explicit that this is read-only, "no auth
// yet (locked to your IP)" — enforced here via ipAllowlist, not a WAF, see
// ipallow.go for why.
package api

import (
	"context"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/config"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/connectors"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/store"
)

// Server holds the API's dependencies. Companies is the data/companies.yaml
// registry loaded once at startup (same source of truth cmd/ingest uses).
// Skills is a snapshot only — handlers read the live-refreshed dictionary
// via skillsMap()/skillByID(), not this field directly, since Phase 5's
// review-queue writeback (docs/design.md §6 Stage 5) means an approved
// skill needs to become visible without a redeploy; see RefreshSkills.
type Server struct {
	Store       *store.Store
	Skills      []model.Skill
	Companies   []connectors.CompanyConfig
	AllowedCIDR string // empty disables IP-restriction (local testing) — see ipallow.go

	yamlSkills []model.Skill // the git-tracked seed RefreshSkills re-merges against each call

	skillsMu   sync.RWMutex
	skillsByID map[string]model.Skill
	tierBySlug map[string]string
}

// New builds a Server, precomputing its lookup maps. skills is the
// yaml-loaded seed; call RefreshSkills afterward (cmd/api does this once at
// startup, then periodically) to merge in any DynamoDB-approved skills.
func New(s *store.Store, skills []model.Skill, companies []connectors.CompanyConfig, allowedCIDR string) *Server {
	skillsByID := make(map[string]model.Skill, len(skills))
	for _, sk := range skills {
		skillsByID[sk.ID] = sk
	}
	tierBySlug := make(map[string]string, len(companies))
	for _, c := range companies {
		tierBySlug[c.Slug] = c.Tier
	}
	return &Server{
		Store:       s,
		Skills:      skills,
		Companies:   companies,
		AllowedCIDR: allowedCIDR,
		yamlSkills:  skills,
		skillsByID:  skillsByID,
		tierBySlug:  tierBySlug,
	}
}

// RefreshSkills re-merges the yaml seed with DynamoDB-approved skills
// (config.MergeSkills) and atomically swaps the lookup map. Safe to call
// concurrently with request handling: readers only ever see either the old
// or the new fully-built map, never a partially-updated one.
func (s *Server) RefreshSkills(ctx context.Context) error {
	dynamic, err := s.Store.ListDynamicSkills(ctx)
	if err != nil {
		return err
	}
	merged := config.MergeSkills(s.yamlSkills, dynamic)
	byID := make(map[string]model.Skill, len(merged))
	for _, sk := range merged {
		byID[sk.ID] = sk
	}

	s.skillsMu.Lock()
	s.Skills = merged
	s.skillsByID = byID
	s.skillsMu.Unlock()
	return nil
}

func (s *Server) skillsMap() map[string]model.Skill {
	s.skillsMu.RLock()
	defer s.skillsMu.RUnlock()
	return s.skillsByID
}

func (s *Server) skillByID(id string) (model.Skill, bool) {
	s.skillsMu.RLock()
	defer s.skillsMu.RUnlock()
	sk, ok := s.skillsByID[id]
	return sk, ok
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
		r.Use(ipAllowlist(s.AllowedCIDR))
		r.Get("/skills", s.handleListSkills)
		r.Get("/skills/{id}", s.handleGetSkill)
		r.Get("/skills/{id}/postings", s.handleGetSkillPostings)
		r.Get("/postings", s.handleListPostings)
		r.Get("/postings/{id}", s.handleGetPosting)
		r.Get("/companies", s.handleListCompanies)
		r.Get("/stats/overview", s.handleStatsOverview)
		r.Get("/reviews", s.handleListReviews)
		r.Post("/reviews/{term}", s.handleReviewAction)
	})

	return r
}
