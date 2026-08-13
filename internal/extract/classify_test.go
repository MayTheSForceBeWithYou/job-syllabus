package extract

import "testing"

func TestClassifyHeading(t *testing.T) {
	cases := []struct {
		heading string
		want    SectionKind
	}{
		{"Requirements", SectionRequirements},
		{"Basic Qualifications", SectionRequirements},
		{"What You'll Need", SectionRequirements},
		{"Preferred Qualifications", SectionNiceToHave}, // must not fall into requirements
		{"Desired Qualifications:", SectionNiceToHave},  // real-world variant found via a live Riot Games posting
		{"Nice to Have", SectionNiceToHave},
		{"Bonus Points", SectionNiceToHave},
		{"Responsibilities", SectionResponsibilities},
		{"What You'll Do", SectionResponsibilities},
		{"About the Role", SectionResponsibilities},
		{"Benefits", SectionBenefits},
		{"Compensation & Perks", SectionBenefits},
		{"Equal Opportunity Employer", SectionBenefits},
		{"About Riot Games", SectionBoilerplate},
		{"", SectionBoilerplate},
		{"Application Process", SectionBoilerplate},
	}

	for _, c := range cases {
		if got := classifyHeading(c.heading); got != c.want {
			t.Errorf("classifyHeading(%q) = %q, want %q", c.heading, got, c.want)
		}
	}
}

func TestWordBoundaryDoesNotMatchInsideWord(t *testing.T) {
	// "plus" is a nice_to_have cue; "Plusieurs" (unrelated word starting
	// with "plus") must not trigger it.
	if wordBoundaryMatch("plusieurs qualifications requises", "plus") {
		t.Error("word boundary match should not match inside an unrelated word")
	}
	if !wordBoundaryMatch("nice-to-haves, plus a car", "plus") {
		t.Error("word boundary match should match a standalone word")
	}
}
