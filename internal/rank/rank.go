// Package rank computes the ranked skill-frequency table from stored
// PostingSkill edges. Shared by cmd/ingest's `report` (text table) and
// internal/api's GET /v1/skills (JSON) so the two never compute this
// differently — extracted here specifically because Phase 3 gave the
// aggregation a second real caller.
package rank

import (
	"sort"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
)

// SkillRank is one row of the ranked table: a skill plus its aggregate
// counts across the edges given to Skills.
type SkillRank struct {
	SkillID    string
	Display    string
	Category   string
	Count      int
	Required   int
	NiceToHave int
}

// Skills aggregates edges into a table sorted by Count descending, then
// SkillID ascending as a stable tie-break. displayByID supplies
// Display/Category; a skill ID missing from it (skills.yaml changed since
// the edge was written) falls back to the raw ID as its display name,
// matching cmd/ingest's existing behavior.
func Skills(edges []model.PostingSkill, displayByID map[string]model.Skill) []SkillRank {
	rows := make(map[string]*SkillRank, len(displayByID))
	for _, e := range edges {
		row, ok := rows[e.SkillID]
		if !ok {
			sk := displayByID[e.SkillID]
			display := sk.Display
			if display == "" {
				display = e.SkillID
			}
			row = &SkillRank{SkillID: e.SkillID, Display: display, Category: sk.Category}
			rows[e.SkillID] = row
		}
		row.Count++
		if e.Required {
			row.Required++
		} else {
			row.NiceToHave++
		}
	}

	out := make([]SkillRank, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].SkillID < out[j].SkillID
	})
	return out
}

// CountCompanies returns the number of distinct CompanySlug values across
// postings — the "(N companies)" figure in both the CLI report and the API.
func CountCompanies(postings []model.Posting) int {
	seen := make(map[string]bool, len(postings))
	for _, p := range postings {
		seen[p.CompanySlug] = true
	}
	return len(seen)
}
