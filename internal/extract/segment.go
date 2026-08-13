package extract

import (
	"strings"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/connectors"
)

const headingPrefix = "## "

// SegmentLines is Stage 2 (Segment) from §6: split Stage-1-normalized lines
// into blocks on "## " heading markers, classify each heading, and apply
// the benefits cutoff — once a block classifies as benefits, that block and
// everything after it become boilerplate, regardless of their own heading
// text, since studios put DEI/perks copy last and it's the biggest noise
// source once you're past it.
func SegmentLines(lines []string) []Section {
	var sections []Section
	cur := Section{Kind: SectionBoilerplate} // unheaded preamble

	flush := func() {
		if len(cur.Lines) > 0 || cur.Heading != "" {
			sections = append(sections, cur)
		}
	}

	for _, line := range lines {
		if heading, ok := strings.CutPrefix(line, headingPrefix); ok {
			flush()
			cur = Section{Kind: classifyHeading(heading), Heading: heading}
			continue
		}
		cur.Lines = append(cur.Lines, line)
	}
	flush()

	pastBenefits := false
	for i := range sections {
		if pastBenefits {
			sections[i].Kind = SectionBoilerplate
			continue
		}
		if sections[i].Kind == SectionBenefits {
			pastBenefits = true
			sections[i].Kind = SectionBoilerplate
		}
	}

	return sections
}

// SegmentPosting runs Stage 1 + Stage 2 for one raw posting. Lever postings
// carry structured `lists` (docs/design.md §5); when present, each list's
// Text is classified as a heading directly and its Content is normalized
// via the same htmlToLines path, skipping the full-body HTML normalize
// entirely per §6 ("use it directly and skip the HTML path"). Any plain
// Description text outside the lists becomes the boilerplate preamble.
// Everything else (Greenhouse and any future HTML-only connector) goes
// through htmlToLines on BodyHTML.
func SegmentPosting(rp connectors.RawPosting) ([]Section, error) {
	if lists, ok := rp.Structured["lists"].([]connectors.LeverList); ok && len(lists) > 0 {
		return segmentLeverLists(rp, lists)
	}

	lines, err := htmlToLines(rp.BodyHTML)
	if err != nil {
		return nil, err
	}
	return SegmentLines(lines), nil
}

func segmentLeverLists(rp connectors.RawPosting, lists []connectors.LeverList) ([]Section, error) {
	var sections []Section

	if preambleLines, err := htmlToLines(rp.BodyHTML); err != nil {
		return nil, err
	} else if len(preambleLines) > 0 {
		sections = append(sections, Section{Kind: SectionBoilerplate, Lines: preambleLines})
	}

	pastBenefits := false
	for _, list := range lists {
		kind := classifyHeading(list.Text)
		if pastBenefits {
			kind = SectionBoilerplate
		} else if kind == SectionBenefits {
			pastBenefits = true
			kind = SectionBoilerplate
		}

		contentLines, err := htmlToLines(list.Content)
		if err != nil {
			return nil, err
		}
		sections = append(sections, Section{Kind: kind, Heading: list.Text, Lines: contentLines})
	}

	return sections, nil
}
