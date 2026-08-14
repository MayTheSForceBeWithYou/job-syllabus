package extract

import (
	"strings"
	"testing"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
)

func testSkills(t *testing.T) []CompiledSkill {
	t.Helper()
	skills := []model.Skill{
		{ID: "python", Display: "Python", Category: "languages", Aliases: []string{"Python"}},
		{ID: "cpp", Display: "C++", Category: "languages", Patterns: []string{`(^|[^\w+])C\+\+([^\w+]|$)`}},
		{ID: "go-lang", Display: "Go", Category: "languages", Patterns: []string{`(?-i)\bGo\b`}},
		{ID: "perforce", Display: "Perforce", Category: "vcs", Aliases: []string{"Perforce", "P4"}},
	}
	compiled, err := CompileSkills(skills)
	if err != nil {
		t.Fatalf("CompileSkills: %v", err)
	}
	return compiled
}

func TestCppPatternMatchesButNotSubstring(t *testing.T) {
	skills := testSkills(t)
	var cpp CompiledSkill
	for _, s := range skills {
		if s.Skill.ID == "cpp" {
			cpp = s
		}
	}

	if _, ok := cpp.findEvidence("5+ years of experience with C++ and Python"); !ok {
		t.Error("expected C++ to match in a normal sentence")
	}
	if _, ok := cpp.findEvidence("Experience with C++."); !ok {
		t.Error("expected C++ to match immediately before a period")
	}
	if _, ok := cpp.findEvidence("no cross-platform experience required"); ok {
		t.Error("C++ pattern should not match unrelated text")
	}
}

func TestGoPatternIsCaseSensitiveAndWordBounded(t *testing.T) {
	skills := testSkills(t)
	var goSkill CompiledSkill
	for _, s := range skills {
		if s.Skill.ID == "go-lang" {
			goSkill = s
		}
	}

	if _, ok := goSkill.findEvidence("Proficiency in Go, Python, and Rust"); !ok {
		t.Error("expected capitalized Go to match")
	}
	if _, ok := goSkill.findEvidence("please go through the onboarding docs"); ok {
		t.Error("lowercase 'go' should not match the Go language pattern")
	}
	if _, ok := goSkill.findEvidence("we use Golang extensively"); ok {
		t.Error("'Golang' should not match a strict word-boundary 'Go' pattern")
	}
}

func TestMatchSkills_RequiredWinsOverNiceToHave(t *testing.T) {
	skills := testSkills(t)
	sections := []Section{
		{Kind: SectionRequirements, Lines: []string{"- 5+ years with Perforce"}},
		{Kind: SectionNiceToHave, Lines: []string{"- Perforce experience is a plus", "- Familiarity with P4"}},
	}

	matches := MatchSkills("posting-1", sections, skills)

	var perforce *model.PostingSkill
	for i := range matches {
		if matches[i].SkillID == "perforce" {
			perforce = &matches[i]
		}
	}
	if perforce == nil {
		t.Fatal("expected a perforce match")
	}
	if !perforce.Required {
		t.Error("perforce appears in a requirements section too — should be Required=true, not overwritten by the nice_to_have mention")
	}
	if perforce.Evidence != "- 5+ years with Perforce" {
		t.Errorf("unexpected evidence: %q", perforce.Evidence)
	}
}

func TestMatchSkills_EvidenceTruncated(t *testing.T) {
	skills := testSkills(t)
	longLine := "- " + strings.Repeat("a", 300) + " Python"
	sections := []Section{{Kind: SectionRequirements, Lines: []string{longLine}}}

	matches := MatchSkills("posting-2", sections, skills)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if len(matches[0].Evidence) > maxEvidenceLen {
		t.Errorf("evidence not truncated: len=%d", len(matches[0].Evidence))
	}
}

func TestMatchSkills_OnlyScansRequirementSections(t *testing.T) {
	skills := testSkills(t)
	sections := []Section{
		{Kind: SectionResponsibilities, Lines: []string{"- Work daily in Perforce and Python"}},
		{Kind: SectionBoilerplate, Lines: []string{"- We use Perforce internally"}},
	}

	matches := MatchSkills("posting-3", sections, skills)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches from non-requirement sections, got %d: %+v", len(matches), matches)
	}
}
