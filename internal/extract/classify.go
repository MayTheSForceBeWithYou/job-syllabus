package extract

import (
	"regexp"
	"strings"
)

// Cue lists from docs/design.md §6, checked in this order (most specific
// first) so e.g. "Preferred Qualifications" lands in nice_to_have rather
// than requirements.
var (
	niceToHaveCues = []string{
		"nice to have", "nice-to-have", "bonus", "preferred", "plus", "pluses",
		"even better", "preferred qualifications",
		// "desired" added after a real Riot Games posting's "Desired
		// Qualifications:" heading matched the bare word "qualifications"
		// in requirementsCues instead — not in §6's original cue list, but
		// a common enough synonym for "preferred" that skipping it would
		// misclassify a real nice-to-have section as required.
		"desired", "desired qualifications",
	}
	requirementsCues = []string{
		"requirements", "requirement", "qualifications", "qualification",
		"what you'll need", "you have", "minimum", "basic qualifications",
	}
	responsibilitiesCues = []string{
		"responsibilities", "responsibility", "what you'll do", "the role", "about the role",
	}
	benefitsCues = []string{
		"benefits", "benefit", "perks", "compensation", "we offer", "equal opportunity", "eeo",
	}
)

var wordBoundaryCache = map[string]*regexp.Regexp{}

// wordBoundaryMatch matches single-word cues on word boundaries (so
// "plus" doesn't match inside an unrelated compound word); multi-word
// phrases are checked as plain substrings by the caller instead, since a
// phrase match is already unambiguous.
func wordBoundaryMatch(haystack, word string) bool {
	re, ok := wordBoundaryCache[word]
	if !ok {
		re = regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
		wordBoundaryCache[word] = re
	}
	return re.MatchString(haystack)
}

func matchesAny(heading string, cues []string) bool {
	for _, cue := range cues {
		if strings.Contains(cue, " ") {
			if strings.Contains(heading, cue) {
				return true
			}
		} else if wordBoundaryMatch(heading, cue) {
			return true
		}
	}
	return false
}

// classifyHeading maps a section heading to a SectionKind. An empty heading
// (the unheaded preamble) and any heading matching none of the cue lists
// both default to boilerplate — conservative on purpose, since unlabeled
// marketing/DEI copy is the extraction pipeline's biggest noise source
// (§6).
//
// KNOWN GAP (accepted, not fixed): this only works for postings that use
// conventional section headings. A real Kabam (Lever) posting used "In
// this role, you can expect to:" and "To be successful in this role, your
// background includes:" — neither matches any cue, so both sections
// default to boilerplate and that posting contributes zero extracted
// skills, despite having real content (Jira, Google Workspace, etc.) in
// the second section. Decision: accept this for now rather than chase
// arbitrary phrasing (see NEXT_STEPS.md). Two paths considered for later:
// (a) a fallback that treats a posting's last substantive non-boilerplate
// section as a lower-confidence requirements candidate when zero sections
// classify normally, or (b) the Stage 5 review queue surfacing terms from
// misclassified sections once Bedrock fallback exists, independent of
// which section they came from.
func classifyHeading(heading string) SectionKind {
	h := strings.ToLower(strings.TrimSpace(heading))
	if h == "" {
		return SectionBoilerplate
	}
	switch {
	case matchesAny(h, niceToHaveCues):
		return SectionNiceToHave
	case matchesAny(h, requirementsCues):
		return SectionRequirements
	case matchesAny(h, responsibilitiesCues):
		return SectionResponsibilities
	case matchesAny(h, benefitsCues):
		return SectionBenefits
	default:
		return SectionBoilerplate
	}
}
