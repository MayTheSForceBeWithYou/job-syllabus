package api

import (
	"net/http"
	"time"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/rank"
)

// handleStatsOverview implements GET /v1/stats/overview — corpus size,
// companies, last run, coverage (docs/design.md §7). "Last run" is
// approximated as the most recent posting LastSeenAt across the corpus —
// there's no dedicated ingest-run-tracking entity yet (see companyDTO's
// comment), but this is a reasonable proxy for "as of when ingest last
// touched anything."
func (s *Server) handleStatsOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	postings, err := s.Store.ListAllPostings(ctx)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to list postings", err.Error())
		return
	}
	edges, err := s.Store.ListAllSkillEdges(ctx)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to list skill edges", err.Error())
		return
	}

	var lastSeen time.Time
	var withSkills int
	for _, p := range postings {
		if p.LastSeenAt.After(lastSeen) {
			lastSeen = p.LastSeenAt
		}
		if p.SkillCount > 0 {
			withSkills++
		}
	}

	coverage := 0.0
	if len(postings) > 0 {
		coverage = float64(withSkills) / float64(len(postings)) * 100
	}

	resp := statsOverviewDTO{
		PostingCount:          len(postings),
		CompanyCount:          rank.CountCompanies(postings),
		SkillEdgeCount:        len(edges),
		DistinctSkillsMatched: len(rank.Skills(edges, s.skillsByID)),
		CoveragePct:           round1(coverage),
	}
	if !lastSeen.IsZero() {
		resp.LastIngestAt = &lastSeen
	}

	writeJSON(w, resp)
}
