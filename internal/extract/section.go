// Package extract implements the posting-body extraction pipeline from
// docs/design.md §6. Currently: Stage 1 (Normalize) and Stage 2 (Segment).
// Stage 3 (dictionary matching) and beyond are not implemented yet.
package extract

// SectionKind classifies one block of a posting body.
type SectionKind string

const (
	SectionResponsibilities SectionKind = "responsibilities"
	SectionRequirements     SectionKind = "requirements"
	SectionNiceToHave       SectionKind = "nice_to_have"
	SectionBenefits         SectionKind = "benefits"
	SectionBoilerplate      SectionKind = "boilerplate"
)

// Section is one classified block of a posting body: a heading (empty for
// the unheaded preamble) plus its content lines. Bullet lines keep their
// "- " prefix; heading lines are not included in Lines.
type Section struct {
	Kind    SectionKind
	Heading string
	Lines   []string
}

// RequirementSections filters to the sections Stage 3 (dictionary matching)
// should scan: requirements and nice_to_have only, per §6 — responsibilities
// and boilerplate are kept for evidence/debugging but not scanned for
// skills.
func RequirementSections(sections []Section) []Section {
	var out []Section
	for _, s := range sections {
		if s.Kind == SectionRequirements || s.Kind == SectionNiceToHave {
			out = append(out, s)
		}
	}
	return out
}
