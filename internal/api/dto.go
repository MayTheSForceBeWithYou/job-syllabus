package api

import (
	"math"
	"strconv"
	"time"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
)

type skillDTO struct {
	ID            string  `json:"id"`
	Display       string  `json:"display"`
	Category      string  `json:"category"`
	Count         int     `json:"count"`
	Required      int     `json:"required"`
	NiceToHave    int     `json:"niceToHave"`
	PctOfPostings float64 `json:"pctOfPostings"`
}

type listSkillsResponse struct {
	Skills        []skillDTO `json:"skills"`
	TotalPostings int        `json:"totalPostings"`
}

// skillDetailDTO deliberately has no trend series — docs/design.md §7 asks
// for one, but that needs time-bucketed history this project doesn't
// track yet (Phase 8, "Trend charts", is where that belongs). ExampleEvidence
// is capped at 3 entries, same rationale as evidence truncation elsewhere:
// enough to show the match is real without dumping the whole corpus.
type skillDetailDTO struct {
	ID              string   `json:"id"`
	Display         string   `json:"display"`
	Category        string   `json:"category"`
	Count           int      `json:"count"`
	Required        int      `json:"required"`
	NiceToHave      int      `json:"niceToHave"`
	ExampleEvidence []string `json:"exampleEvidence"`
}

type postingSummaryDTO struct {
	ID          string    `json:"id"`
	CompanySlug string    `json:"companySlug"`
	Title       string    `json:"title"`
	RoleFamily  string    `json:"roleFamily"`
	Location    string    `json:"location"`
	URL         string    `json:"url"`
	Source      string    `json:"source"`
	PostedAt    time.Time `json:"postedAt"`
	SkillCount  int       `json:"skillCount"`
}

func toPostingSummary(p model.Posting) postingSummaryDTO {
	return postingSummaryDTO{
		ID:          p.ID,
		CompanySlug: p.CompanySlug,
		Title:       p.Title,
		RoleFamily:  string(p.RoleFamily),
		Location:    p.Location,
		URL:         p.URL,
		Source:      p.Source,
		PostedAt:    p.PostedAt,
		SkillCount:  p.SkillCount,
	}
}

type listPostingsResponse struct {
	Postings   []postingSummaryDTO `json:"postings"`
	NextCursor string              `json:"nextCursor,omitempty"`
}

type postingSkillDTO struct {
	SkillID    string  `json:"skillId"`
	Display    string  `json:"display"`
	Required   bool    `json:"required"`
	Evidence   string  `json:"evidence"`
	Confidence float32 `json:"confidence"`
	Method     string  `json:"method"`
}

type postingDetailDTO struct {
	postingSummaryDTO
	Skills []postingSkillDTO `json:"skills"`
}

// companyDTO omits "last ingest status" from docs/design.md §7's spec —
// no ingest-run-tracking entity exists yet (nothing writes a Company item
// or records per-run outcomes); PostingCount is computed live from stored
// postings instead. Real ingest-status tracking is a Phase 4 concern
// ("Ingestion at scale" — daily runs green, DLQ empty).
type companyDTO struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Tier         string `json:"tier"`
	ATS          string `json:"ats"`
	PostingCount int    `json:"postingCount"`
}

type listCompaniesResponse struct {
	Companies []companyDTO `json:"companies"`
}

type statsOverviewDTO struct {
	PostingCount          int        `json:"postingCount"`
	CompanyCount          int        `json:"companyCount"`
	SkillEdgeCount        int        `json:"skillEdgeCount"`
	DistinctSkillsMatched int        `json:"distinctSkillsMatched"`
	LastIngestAt          *time.Time `json:"lastIngestAt,omitempty"`
	CoveragePct           float64    `json:"coveragePct"`
}

// parseIntParam parses a query param as a positive int, clamped to
// [1, max], falling back to def on an empty or invalid value rather than
// erroring — limit/offset-style params are the one place a client typo
// shouldn't turn into a 400.
func parseIntParam(raw string, def, max int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func round1(f float64) float64 {
	return math.Round(f*10) / 10
}

// reviewDTO is one pending unknown term (docs/design.md §7 GET /v1/reviews:
// "Pending unknown terms, sorted by frequency").
type reviewDTO struct {
	Term        string   `json:"term"`
	Category    string   `json:"category"`
	Occurrences int      `json:"occurrences"`
	Evidence    []string `json:"evidence"`
}

type listReviewsResponse struct {
	Reviews []reviewDTO `json:"reviews"`
}

// reviewActionRequest is POST /v1/reviews/{term}'s body (docs/design.md §7:
// "{action: create|alias|reject, ...}"). Fields are action-specific:
//   - create: display, category, aliases (skillId defaults to a slug of the
//     term if omitted)
//   - alias: mergeIntoSkillId
//   - reject: no extra fields
type reviewActionRequest struct {
	Action           string   `json:"action"`
	SkillID          string   `json:"skillId,omitempty"`
	Display          string   `json:"display,omitempty"`
	Category         string   `json:"category,omitempty"`
	Aliases          []string `json:"aliases,omitempty"`
	MergeIntoSkillID string   `json:"mergeIntoSkillId,omitempty"`
}

type reviewActionResponse struct {
	Term   string    `json:"term"`
	Action string    `json:"action"`
	Skill  *skillDTO `json:"skill,omitempty"`
}
