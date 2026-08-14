package rank

import (
	"testing"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
)

func TestSkillsRanksByCountThenID(t *testing.T) {
	edges := []model.PostingSkill{
		{PostingID: "p1", SkillID: "go", Required: true},
		{PostingID: "p2", SkillID: "go", Required: false},
		{PostingID: "p1", SkillID: "aws", Required: true},
		{PostingID: "p2", SkillID: "aws", Required: true},
		{PostingID: "p3", SkillID: "aws", Required: false},
		{PostingID: "p1", SkillID: "zig", Required: true},
		{PostingID: "p3", SkillID: "zig", Required: false}, // zig count=2, ties with go — alphabetical tie-break decides order
	}
	displayByID := map[string]model.Skill{
		"go":  {ID: "go", Display: "Go", Category: "languages"},
		"aws": {ID: "aws", Display: "AWS", Category: "cloud"},
		// "zig" deliberately absent — must fall back to raw ID.
	}

	rows := Skills(edges, displayByID)

	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	// aws: 3 edges (2 required, 1 nice-to-have) — highest count, sorts first.
	if rows[0].SkillID != "aws" || rows[0].Count != 3 || rows[0].Required != 2 || rows[0].NiceToHave != 1 {
		t.Errorf("row 0 = %+v, want aws count=3 required=2 niceToHave=1", rows[0])
	}
	// go and zig both have count=2 — tie-break alphabetically by SkillID.
	if rows[1].SkillID != "go" || rows[1].Display != "Go" {
		t.Errorf("row 1 = %+v, want go", rows[1])
	}
	if rows[2].SkillID != "zig" || rows[2].Display != "zig" {
		t.Errorf("row 2 = %+v, want zig with fallback display", rows[2])
	}
}

func TestCountCompaniesDeduplicates(t *testing.T) {
	postings := []model.Posting{
		{CompanySlug: "epic-games"},
		{CompanySlug: "epic-games"},
		{CompanySlug: "riot-games"},
	}
	if got := CountCompanies(postings); got != 2 {
		t.Errorf("CountCompanies() = %d, want 2", got)
	}
}
