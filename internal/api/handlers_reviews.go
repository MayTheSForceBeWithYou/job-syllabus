package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/config"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/store"
)

// handleListReviews is Stage 5's read path (docs/design.md §7 GET
// /v1/reviews): every Bedrock-discovered term that didn't resolve to a
// known skill, sorted by how often it's been seen — cmd/worker writes these
// via internal/store.RecordReviewOccurrence.
func (s *Server) handleListReviews(w http.ResponseWriter, r *http.Request) {
	reviews, err := s.Store.ListPendingReviews(r.Context())
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to list reviews", err.Error())
		return
	}

	resp := make([]reviewDTO, 0, len(reviews))
	for _, rv := range reviews {
		resp = append(resp, reviewDTO{
			Term:        rv.Term,
			Category:    rv.Category,
			Occurrences: rv.Occurrences,
			Evidence:    rv.Evidence,
		})
	}
	writeJSON(w, listReviewsResponse{Reviews: resp})
}

// handleReviewAction is Stage 5's write path (docs/design.md §7 POST
// /v1/reviews/{term}): triage one pending term as create (new canonical
// skill), alias (merge into an existing skill), or reject (noise). The
// design's prose describes this opening a git commit against
// data/skills.yaml; this project has no GitHub write credential configured
// (see docs/phase-5.md), so create/alias instead write straight to
// DynamoDB (internal/store.PutSkill) and RefreshSkills immediately makes
// the change visible on this same server — the git-tracked yaml file stays
// the seed, synced by hand from DynamoDB periodically, not on every
// approval.
func (s *Server) handleReviewAction(w http.ResponseWriter, r *http.Request) {
	// The {term} path segment is base64url (no padding), not a plain
	// URL-encoded string — a term containing "/" (e.g. "CI/CD") broke this
	// route even with the client correctly calling encodeURIComponent:
	// API Gateway HTTP APIs unconditionally decode %2F back into a literal
	// "/" before forwarding the path, which chi's single-segment {term}
	// route can't match. Base64url's alphabet has no "/" or "+", so it
	// survives the trip regardless of what the term contains.
	rawTerm := chi.URLParam(r, "term")
	decoded, err := base64.RawURLEncoding.DecodeString(rawTerm)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid term", "term path segment must be base64url-encoded (no padding): "+err.Error())
		return
	}
	term := store.NormalizeTerm(string(decoded))
	if term == "" {
		writeProblem(w, r, http.StatusBadRequest, "invalid term", "term must not be empty")
		return
	}

	var req reviewActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	ctx := r.Context()
	review, err := s.Store.GetPendingReview(ctx, term)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to load review", err.Error())
		return
	}
	if review == nil {
		writeProblem(w, r, http.StatusNotFound, "no pending review for this term", "term="+term+" is not (or is no longer) in the review queue")
		return
	}

	switch req.Action {
	case "create":
		s.handleCreateSkill(w, r, *review, req)
	case "alias":
		s.handleAliasSkill(w, r, *review, req)
	case "reject":
		s.handleRejectTerm(w, r, *review)
	default:
		writeProblem(w, r, http.StatusBadRequest, "invalid action", `action must be one of "create", "alias", "reject"`)
	}
}

func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request, review store.ReviewTerm, req reviewActionRequest) {
	ctx := r.Context()

	skillID := req.SkillID
	if skillID == "" {
		skillID = store.SlugifyTerm(review.Term)
	}
	if !config.ValidSkillID(skillID) {
		writeProblem(w, r, http.StatusBadRequest, "invalid skill id", "skillId must match ^[a-z0-9-]+$ (got "+skillID+")")
		return
	}
	if _, exists := s.skillByID(skillID); exists {
		writeProblem(w, r, http.StatusConflict, "skill id already exists", "skillId "+skillID+" already exists — use action=alias to merge this term into it instead")
		return
	}

	display := req.Display
	if display == "" {
		display = review.Term
	}
	category := req.Category
	if category == "" {
		category = review.Category
	}
	if category == "" {
		writeProblem(w, r, http.StatusBadRequest, "category required", "category is required when the review item has no Bedrock-suggested category either")
		return
	}
	aliases := req.Aliases
	if len(aliases) == 0 {
		aliases = []string{review.Term}
	}

	newSkill := model.Skill{ID: skillID, Display: display, Category: category, Aliases: aliases}
	if err := s.Store.PutSkill(ctx, newSkill); err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to create skill", err.Error())
		return
	}
	if err := s.Store.ResolvePendingReview(ctx, review.Term); err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "skill created but failed to clear review queue entry", err.Error())
		return
	}
	if err := s.RefreshSkills(ctx); err != nil {
		// Not fatal — the periodic reload will pick it up within
		// skillReloadInterval either way; this just makes the DoD's
		// "approving one via API updates the dictionary" observable
		// immediately on this same connection.
		writeProblem(w, r, http.StatusInternalServerError, "skill created but dictionary refresh failed", err.Error())
		return
	}

	dto := skillDTO{ID: newSkill.ID, Display: newSkill.Display, Category: newSkill.Category}
	writeJSON(w, reviewActionResponse{Term: review.Term, Action: "create", Skill: &dto})
}

func (s *Server) handleAliasSkill(w http.ResponseWriter, r *http.Request, review store.ReviewTerm, req reviewActionRequest) {
	ctx := r.Context()

	if req.MergeIntoSkillID == "" {
		writeProblem(w, r, http.StatusBadRequest, "mergeIntoSkillId required", "action=alias needs mergeIntoSkillId naming the existing skill to merge this term into")
		return
	}
	target, ok := s.skillByID(req.MergeIntoSkillID)
	if !ok {
		writeProblem(w, r, http.StatusNotFound, "target skill not found", "no skill with id "+req.MergeIntoSkillID)
		return
	}

	target.Aliases = appendUniqueAlias(target.Aliases, review.Term)
	if err := s.Store.PutSkill(ctx, target); err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to update skill aliases", err.Error())
		return
	}
	if err := s.Store.ResolvePendingReview(ctx, review.Term); err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "alias added but failed to clear review queue entry", err.Error())
		return
	}
	if err := s.RefreshSkills(ctx); err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "alias added but dictionary refresh failed", err.Error())
		return
	}

	dto := skillDTO{ID: target.ID, Display: target.Display, Category: target.Category}
	writeJSON(w, reviewActionResponse{Term: review.Term, Action: "alias", Skill: &dto})
}

func (s *Server) handleRejectTerm(w http.ResponseWriter, r *http.Request, review store.ReviewTerm) {
	ctx := r.Context()
	if err := s.Store.RejectTerm(ctx, review.Term); err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to reject term", err.Error())
		return
	}
	if err := s.Store.ResolvePendingReview(ctx, review.Term); err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "term rejected but failed to clear review queue entry", err.Error())
		return
	}
	writeJSON(w, reviewActionResponse{Term: review.Term, Action: "reject"})
}

func appendUniqueAlias(aliases []string, term string) []string {
	for _, a := range aliases {
		if a == term {
			return aliases
		}
	}
	return append(aliases, term)
}
