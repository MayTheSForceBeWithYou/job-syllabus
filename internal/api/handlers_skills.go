package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/rank"
)

// handleListSkills is the "money endpoint" (docs/design.md §7): ranked
// skill frequency. Params: roleFamily, tier, required (true/false/both),
// since, limit. Filtering happens over the whole corpus rather than via a
// GSI query — same "fine for a few-hundred-item corpus" tradeoff as
// ListAllPostings/ListAllSkillEdges (docs/design.md Phase 4 revisits this
// at scale).
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	var since time.Time
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "invalid since parameter", "since must be RFC3339, e.g. 2026-01-01T00:00:00Z")
			return
		}
		since = t
	}
	roleFilter := q.Get("roleFamily")
	tierFilter := q.Get("tier")
	requiredFilter := q.Get("required") // "true" | "false" | "" (both)

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

	// Closed postings are always excluded here, unconditionally — not a
	// user-facing filter param but the definition of "active" (docs/
	// design.md §4: a posting disappearing from an ATS feed means it
	// closed, and gets excluded from active stats while staying in the
	// store for historical trend queries).
	allowed := make(map[string]bool, len(postings))
	postingCount := 0
	for _, p := range postings {
		if p.ClosedAt != nil {
			continue
		}
		if roleFilter != "" && string(p.RoleFamily) != roleFilter {
			continue
		}
		if tierFilter != "" && s.tierBySlug[p.CompanySlug] != tierFilter {
			continue
		}
		if !since.IsZero() && p.PostedAt.Before(since) {
			continue
		}
		allowed[p.ID] = true
		postingCount++
	}

	kept := make([]model.PostingSkill, 0, len(edges))
	for _, e := range edges {
		if !allowed[e.PostingID] {
			continue
		}
		if requiredFilter == "true" && !e.Required {
			continue
		}
		if requiredFilter == "false" && e.Required {
			continue
		}
		kept = append(kept, e)
	}

	rows := rank.Skills(kept, s.skillsMap())

	limit := parseIntParam(q.Get("limit"), 100, 500)
	if len(rows) > limit {
		rows = rows[:limit]
	}

	resp := make([]skillDTO, 0, len(rows))
	for _, row := range rows {
		pct := 0.0
		if postingCount > 0 {
			pct = float64(row.Count) / float64(postingCount) * 100
		}
		resp = append(resp, skillDTO{
			ID:            row.SkillID,
			Display:       row.Display,
			Category:      row.Category,
			Count:         row.Count,
			Required:      row.Required,
			NiceToHave:    row.NiceToHave,
			PctOfPostings: round1(pct),
		})
	}

	writeJSON(w, listSkillsResponse{Skills: resp, TotalPostings: postingCount})
}

// handleGetSkill returns skill detail + example evidence. No trend series
// — see skillDetailDTO's comment.
func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sk, ok := s.skillByID(id)
	if !ok {
		writeProblem(w, r, http.StatusNotFound, "skill not found", "no skill with id "+id+" in the dictionary")
		return
	}

	edges, err := s.Store.ListSkillEdgesForSkill(r.Context(), id)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to list skill edges", err.Error())
		return
	}

	var required, niceToHave int
	evidence := make([]string, 0, 3)
	for _, e := range edges {
		if e.Required {
			required++
		} else {
			niceToHave++
		}
		if e.Evidence != "" && len(evidence) < 3 {
			evidence = append(evidence, e.Evidence)
		}
	}

	writeJSON(w, skillDetailDTO{
		ID:              sk.ID,
		Display:         sk.Display,
		Category:        sk.Category,
		Count:           len(edges),
		Required:        required,
		NiceToHave:      niceToHave,
		ExampleEvidence: evidence,
	})
}

// handleGetSkillPostings lists postings mentioning a skill. This does one
// GetPosting per edge (N+1) rather than a batch fetch — acceptable at
// Phase 3's few-hundred-posting scale; BatchGetItem is the fix once that
// stops being true (docs/design.md Phase 4).
func (s *Server) handleGetSkillPostings(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := s.skillByID(id); !ok {
		writeProblem(w, r, http.StatusNotFound, "skill not found", "no skill with id "+id+" in the dictionary")
		return
	}

	edges, err := s.Store.ListSkillEdgesForSkill(r.Context(), id)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to list skill edges", err.Error())
		return
	}

	postings := make([]postingSummaryDTO, 0, len(edges))
	for _, e := range edges {
		p, err := s.Store.GetPosting(r.Context(), e.PostingID)
		if err != nil {
			writeProblem(w, r, http.StatusInternalServerError, "failed to load posting", err.Error())
			return
		}
		if p == nil {
			continue // edge outlived its posting — shouldn't happen, but don't 500 over it
		}
		postings = append(postings, toPostingSummary(*p))
	}

	writeJSON(w, listPostingsResponse{Postings: postings})
}
