package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/store"
)

// handleListPostings implements GET /v1/postings — params company,
// roleFamily, since, cursor, limit (docs/design.md §7).
func (s *Server) handleListPostings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := store.PostingFilter{
		Company:    q.Get("company"),
		RoleFamily: q.Get("roleFamily"),
		Cursor:     q.Get("cursor"),
	}
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "invalid since parameter", "since must be RFC3339, e.g. 2026-01-01T00:00:00Z")
			return
		}
		filter.Since = t
	}
	if v := q.Get("limit"); v != "" {
		n := parseIntParam(v, 0, 100)
		if n == 0 {
			writeProblem(w, r, http.StatusBadRequest, "invalid limit parameter", "limit must be a positive integer")
			return
		}
		filter.Limit = int32(n)
	}

	page, err := s.Store.ListPostings(r.Context(), filter)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to list postings", err.Error())
		return
	}

	resp := make([]postingSummaryDTO, 0, len(page.Postings))
	for _, p := range page.Postings {
		resp = append(resp, toPostingSummary(p))
	}
	writeJSON(w, listPostingsResponse{Postings: resp, NextCursor: page.NextCursor})
}

// handleGetPosting implements GET /v1/postings/{id} — detail including
// extracted skills with evidence.
func (s *Server) handleGetPosting(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	p, err := s.Store.GetPosting(r.Context(), id)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to load posting", err.Error())
		return
	}
	if p == nil {
		writeProblem(w, r, http.StatusNotFound, "posting not found", "no posting with id "+id)
		return
	}

	edges, err := s.Store.ListSkillEdgesForPosting(r.Context(), id)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to list skill edges", err.Error())
		return
	}

	skills := make([]postingSkillDTO, 0, len(edges))
	for _, e := range edges {
		display := e.SkillID
		if sk, ok := s.skillByID(e.SkillID); ok && sk.Display != "" {
			display = sk.Display
		}
		skills = append(skills, postingSkillDTO{
			SkillID:    e.SkillID,
			Display:    display,
			Required:   e.Required,
			Evidence:   e.Evidence,
			Confidence: e.Confidence,
			Method:     e.Method,
		})
	}

	writeJSON(w, postingDetailDTO{
		postingSummaryDTO: toPostingSummary(*p),
		Skills:            skills,
	})
}
