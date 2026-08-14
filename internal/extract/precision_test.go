package extract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/config"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/connectors"
)

// minPrecision is the hard gate from docs/design.md §6 ("Validation"):
// "fails the build below 0.90 precision". Recall has no specified floor in
// the doc — it's logged for visibility, not asserted, since low recall on
// postings hit by the accepted heading-classification gap (see
// classifyHeading's KNOWN GAP comment) is expected and shouldn't fail the
// build for a known, documented limitation.
const minPrecision = 0.90

type expectedSkill struct {
	SkillID  string `json:"skillId"`
	Required bool   `json:"required"`
}

type labeledFixture struct {
	PostingID  string `json:"postingId"`
	Title      string `json:"title"`
	Company    string `json:"company"`
	RawPosting struct {
		BodyHTML   string                 `json:"bodyHtml"`
		LeverLists []connectors.LeverList `json:"leverLists,omitempty"`
	} `json:"rawPosting"`
	ExpectedSkills []expectedSkill `json:"expectedSkills"`
}

func loadFixtures(t *testing.T) []labeledFixture {
	t.Helper()
	paths, err := filepath.Glob("../../testdata/labeled/*.json")
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no fixtures found in testdata/labeled — validation set is missing")
	}

	fixtures := make([]labeledFixture, 0, len(paths))
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		var f labeledFixture
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		fixtures = append(fixtures, f)
	}
	return fixtures
}

// TestExtractionPrecisionRecall is the §6 Validation gate: hand-labeled
// postings run through the real Stage 1-3 pipeline against the real
// data/skills.yaml, checked against ground truth. Fails the build below
// 90% precision.
func TestExtractionPrecisionRecall(t *testing.T) {
	fixtures := loadFixtures(t)

	rawSkills, err := config.LoadSkills("../../data/skills.yaml")
	if err != nil {
		t.Fatalf("load skills.yaml: %v", err)
	}
	skills, err := CompileSkills(rawSkills)
	if err != nil {
		t.Fatalf("compile skills: %v", err)
	}

	var truePositives, falsePositives, falseNegatives int
	var falsePositiveDetails, falseNegativeDetails []string

	for _, f := range fixtures {
		rp := connectors.RawPosting{BodyHTML: f.RawPosting.BodyHTML}
		if len(f.RawPosting.LeverLists) > 0 {
			rp.Structured = map[string]any{"lists": f.RawPosting.LeverLists}
		}

		sections, err := SegmentPosting(rp)
		if err != nil {
			t.Fatalf("segment fixture %s: %v", f.PostingID, err)
		}
		matches := MatchSkills(f.PostingID, sections, skills)

		predicted := make(map[string]bool, len(matches))
		for _, m := range matches {
			predicted[m.SkillID] = true
		}
		expected := make(map[string]bool, len(f.ExpectedSkills))
		for _, e := range f.ExpectedSkills {
			expected[e.SkillID] = true
		}

		for id := range predicted {
			if expected[id] {
				truePositives++
			} else {
				falsePositives++
				falsePositiveDetails = append(falsePositiveDetails, f.Company+"/"+f.PostingID+": "+id)
			}
		}
		for id := range expected {
			if !predicted[id] {
				falseNegatives++
				falseNegativeDetails = append(falseNegativeDetails, f.Company+"/"+f.PostingID+": "+id)
			}
		}
	}

	total := truePositives + falsePositives
	var precision float64
	if total > 0 {
		precision = float64(truePositives) / float64(total)
	}
	recallTotal := truePositives + falseNegatives
	var recall float64
	if recallTotal > 0 {
		recall = float64(truePositives) / float64(recallTotal)
	}

	t.Logf("fixtures=%d truePositives=%d falsePositives=%d falseNegatives=%d precision=%.3f recall=%.3f",
		len(fixtures), truePositives, falsePositives, falseNegatives, precision, recall)
	if len(falsePositiveDetails) > 0 {
		t.Logf("false positives:\n  %s", joinLines(falsePositiveDetails))
	}
	if len(falseNegativeDetails) > 0 {
		t.Logf("false negatives (expected recall gaps, e.g. the accepted heading-classification limitation):\n  %s", joinLines(falseNegativeDetails))
	}

	if precision < minPrecision {
		t.Errorf("precision %.3f is below the %.2f gate from docs/design.md §6 — false positives:\n  %s",
			precision, minPrecision, joinLines(falsePositiveDetails))
	}
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n  "
		}
		out += l
	}
	return out
}
