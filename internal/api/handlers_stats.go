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

	allPostings, err := s.Store.ListAllPostings(ctx)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to list postings", err.Error())
		return
	}
	allEdges, err := s.Store.ListAllSkillEdges(ctx)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to list skill edges", err.Error())
		return
	}

	// Active only (docs/design.md §4): a closed posting isn't deleted, but
	// it drops out of every stat here just as it does from GET /v1/skills
	// — this is the same corpus definition "500+ postings"-style DoD
	// language and cmd/rollup's reconciliation both mean.
	active := make(map[string]bool, len(allPostings))
	postings := allPostings[:0:0]
	for _, p := range allPostings {
		if p.ClosedAt != nil {
			continue
		}
		active[p.ID] = true
		postings = append(postings, p)
	}
	edges := allEdges[:0:0]
	for _, e := range allEdges {
		if active[e.PostingID] {
			edges = append(edges, e)
		}
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
