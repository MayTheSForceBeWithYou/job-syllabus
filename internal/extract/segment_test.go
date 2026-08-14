package extract

import (
	"reflect"
	"testing"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/connectors"
)

const samplePostingHTML = `
<div>
  <p>We are looking for a Build Engineer to join our platform team.</p>
  <h2>About the Role</h2>
  <p>You will own our build pipeline and keep it fast.</p>
  <h2>Requirements</h2>
  <ul>
    <li>5+ years of experience with Perforce</li>
    <li>Proficiency in Python and Go</li>
  </ul>
  <h2>Nice to Have</h2>
  <ul>
    <li>Experience with Jenkins</li>
  </ul>
  <h2>Benefits</h2>
  <p>Health insurance, 401k, and more.</p>
  <h2>Equal Opportunity Employer</h2>
  <p>We do not discriminate.</p>
</div>`

// samplePostingBoldHeadings mirrors a real Roblox posting structure: no
// semantic <h*> tags for the requirements/nice-to-have split at all, just
// bolded paragraphs (<p><strong>...</strong></p>).
const samplePostingBoldHeadings = `
<div>
  <p>Some intro copy about the team.</p>
  <h3>Senior Engineer, Platform</h3>
  <p>Role description paragraph.</p>
  <p><strong>You Will</strong></p>
  <ul><li>Own the deployment pipeline</li></ul>
  <p><strong>You Have</strong></p>
  <ul>
    <li>Experience with Kubernetes</li>
    <li>Proficiency in Go</li>
  </ul>
  <p>This paragraph <strong>starts</strong> with bold but isn't a heading.</p>
</div>`

func TestSegmentPosting_BoldParagraphPseudoHeadings(t *testing.T) {
	rp := connectors.RawPosting{BodyHTML: samplePostingBoldHeadings}

	sections, err := SegmentPosting(rp)
	if err != nil {
		t.Fatalf("SegmentPosting: %v", err)
	}

	var youHave *Section
	for i := range sections {
		if sections[i].Heading == "You Have" {
			youHave = &sections[i]
		}
	}
	if youHave == nil {
		t.Fatalf("expected a 'You Have' section from the bolded paragraph; got sections: %+v", sections)
	}
	if youHave.Kind != SectionRequirements {
		t.Errorf("'You Have' kind = %q, want requirements", youHave.Kind)
	}
	if len(youHave.Lines) < 2 {
		t.Errorf("'You Have' lines = %v, want at least 2 bullets", youHave.Lines)
	}
	if youHave.Lines[0] != "- Experience with Kubernetes" || youHave.Lines[1] != "- Proficiency in Go" {
		t.Errorf("unexpected 'You Have' bullets: %v", youHave.Lines)
	}

	// The paragraph that merely starts with bold text must NOT be split
	// into its own heading section — it should fold into "You Have" as
	// plain content instead (confirmed above: 3 lines, not 2).
	for _, s := range sections {
		if s.Heading == "starts" || s.Heading == "This paragraph" {
			t.Errorf("partially-bold paragraph was wrongly treated as a heading: %+v", s)
		}
	}
}

func TestSegmentPosting_HTMLBenefitsCutoff(t *testing.T) {
	rp := connectors.RawPosting{BodyHTML: samplePostingHTML}

	sections, err := SegmentPosting(rp)
	if err != nil {
		t.Fatalf("SegmentPosting: %v", err)
	}

	var kinds []SectionKind
	for _, s := range sections {
		kinds = append(kinds, s.Kind)
	}
	want := []SectionKind{
		SectionBoilerplate,      // preamble
		SectionResponsibilities, // About the Role
		SectionRequirements,
		SectionNiceToHave,
		SectionBoilerplate, // Benefits itself
		SectionBoilerplate, // EEO — forced boilerplate by the cutoff, not its own heading match
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("section kinds = %v, want %v", kinds, want)
	}

	reqSections := RequirementSections(sections)
	if len(reqSections) != 2 {
		t.Fatalf("RequirementSections returned %d sections, want 2", len(reqSections))
	}
	if reqSections[0].Lines[0] != "- 5+ years of experience with Perforce" {
		t.Errorf("unexpected requirements content: %q", reqSections[0].Lines[0])
	}
	if reqSections[1].Lines[0] != "- Experience with Jenkins" {
		t.Errorf("unexpected nice_to_have content: %q", reqSections[1].Lines[0])
	}
}

func TestSegmentPosting_LeverLists(t *testing.T) {
	rp := connectors.RawPosting{
		BodyHTML: "<p>Join our live ops team.</p>",
		Structured: map[string]any{
			"lists": []connectors.LeverList{
				{Text: "Requirements", Content: "<ul><li>Strong SQL skills</li></ul>"},
				{Text: "Bonus", Content: "<ul><li>AWS experience</li></ul>"},
			},
		},
	}

	sections, err := SegmentPosting(rp)
	if err != nil {
		t.Fatalf("SegmentPosting: %v", err)
	}
	if len(sections) != 3 {
		t.Fatalf("got %d sections, want 3 (preamble + 2 lists)", len(sections))
	}
	if sections[0].Kind != SectionBoilerplate {
		t.Errorf("preamble kind = %q, want boilerplate", sections[0].Kind)
	}
	if sections[1].Kind != SectionRequirements || sections[1].Lines[0] != "- Strong SQL skills" {
		t.Errorf("requirements list not parsed correctly: %+v", sections[1])
	}
	if sections[2].Kind != SectionNiceToHave || sections[2].Lines[0] != "- AWS experience" {
		t.Errorf("bonus list not parsed correctly: %+v", sections[2])
	}
}
