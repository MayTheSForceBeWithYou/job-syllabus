package extract

import (
	"fmt"
	"regexp"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
)

// Version is stamped onto Posting.ExtractVer. Bump it whenever skills.yaml
// or the matching/segmentation logic changes materially, so
// re-extraction can be targeted at postings with ExtractVer < Version
// (docs/design.md §6 "Re-extraction") — not wired up yet since there's no
// re-extraction job in Phase 1, but the field is meaningful from the start.
const Version = 1

const maxEvidenceLen = 200

// CompiledSkill pairs a canonical skill with its compiled matchers.
type CompiledSkill struct {
	Skill    model.Skill
	patterns []*regexp.Regexp
}

// CompileSkills builds matchers for every skill: each alias becomes a
// case-insensitive, word-boundary regex (`\bAlias\b`); each explicit
// pattern is compiled as given, with no automatic wrapping — that's the
// escape hatch for tokens \b doesn't handle correctly, e.g. `\b` doesn't
// match between "++" and a following space (both non-word characters), so
// "C++" needs `patterns` with an explicit boundary, not an alias. Same
// idea for "Go": the bare word "go" appears constantly as a verb, so
// aliasing it case-insensitively would flag nearly every posting; it's
// seeded as a pattern with `(?-i)` to require the capitalized form.
func CompileSkills(skills []model.Skill) ([]CompiledSkill, error) {
	compiled := make([]CompiledSkill, 0, len(skills))
	for _, s := range skills {
		if s.Deprecated {
			continue
		}
		var patterns []*regexp.Regexp

		for _, alias := range s.Aliases {
			re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(alias) + `\b`)
			if err != nil {
				return nil, fmt.Errorf("skill %q: compile alias %q: %w", s.ID, alias, err)
			}
			patterns = append(patterns, re)
		}
		for _, pat := range s.Patterns {
			re, err := regexp.Compile(pat)
			if err != nil {
				return nil, fmt.Errorf("skill %q: compile pattern %q: %w", s.ID, pat, err)
			}
			patterns = append(patterns, re)
		}

		compiled = append(compiled, CompiledSkill{Skill: s, patterns: patterns})
	}
	return compiled, nil
}

func (c CompiledSkill) findEvidence(line string) (string, bool) {
	for _, re := range c.patterns {
		if loc := re.FindStringIndex(line); loc != nil {
			evidence := line
			if len(evidence) > maxEvidenceLen {
				evidence = evidence[:maxEvidenceLen]
			}
			return evidence, true
		}
	}
	return "", false
}

// MatchSkills is Stage 3 (dictionary pass) from §6: scans a posting's
// requirements and nice_to_have sections (via RequirementSections) for
// each compiled skill and returns one PostingSkill edge per match.
//
// A skill mentioned in both a requirements section and a nice_to_have
// section of the same posting gets a single edge with Required=true —
// "required" is the stronger signal and PostingSkill has exactly one
// Required value per (posting, skill) pair, so it wins rather than being
// overwritten by whichever section happened to be scanned last.
func MatchSkills(postingID string, sections []Section, skills []CompiledSkill) []model.PostingSkill {
	required := make(map[string]model.PostingSkill)
	niceToHave := make(map[string]model.PostingSkill)

	for _, sec := range RequirementSections(sections) {
		dest := niceToHave
		if sec.Kind == SectionRequirements {
			dest = required
		}
		for _, cs := range skills {
			if _, already := dest[cs.Skill.ID]; already {
				continue
			}
			for _, line := range sec.Lines {
				if evidence, ok := cs.findEvidence(line); ok {
					dest[cs.Skill.ID] = model.PostingSkill{
						PostingID:  postingID,
						SkillID:    cs.Skill.ID,
						Required:   sec.Kind == SectionRequirements,
						Evidence:   evidence,
						Confidence: 1.0,
						Method:     "dict",
					}
					break
				}
			}
		}
	}

	matches := make([]model.PostingSkill, 0, len(required)+len(niceToHave))
	for id, m := range required {
		matches = append(matches, m)
		delete(niceToHave, id) // required wins over a nice_to_have match of the same skill
	}
	for _, m := range niceToHave {
		matches = append(matches, m)
	}
	return matches
}
