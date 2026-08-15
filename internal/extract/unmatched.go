package extract

import "strings"

// UnmatchedBullet is one requirement/nice_to_have line the dictionary pass
// (Stage 3) found zero skill matches in — a candidate for Stage 4's Bedrock
// fallback (docs/design.md §6: "Only for requirement/nice-to-have bullets
// with zero dictionary hits").
type UnmatchedBullet struct {
	Text     string
	Required bool
}

// minBulletLen filters out bullets too short to plausibly name a skill
// (stray punctuation, a lone "-" from a malformed list) — not in the design
// doc explicitly, but there's no reason to spend a Bedrock call (even a
// free-tier-cheap one) or a cache slot on them.
const minBulletLen = 3

// FindUnmatchedBullets re-scans the same sections MatchSkills already
// looked at and returns every requirement/nice_to_have line none of the
// compiled skills matched, deduped by exact text within this posting (the
// same phrasing repeated across a "Requirements" and "Nice to have"
// section, or within one section's bullet list, only needs one Bedrock
// classification — Required reflects the section it was first seen in).
//
// This re-runs the same per-line matching MatchSkills does rather than
// having MatchSkills report its own misses — a small amount of duplicated
// regex work against a Phase-1-scale corpus, traded for keeping Stage 3
// (dictionary matching, covered by the §6 precision/recall gate) and Stage
// 4's handoff logic independently readable and testable.
func FindUnmatchedBullets(sections []Section, skills []CompiledSkill) []UnmatchedBullet {
	var out []UnmatchedBullet
	seen := make(map[string]bool)

	for _, sec := range RequirementSections(sections) {
		for _, line := range sec.Lines {
			trimmed := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			trimmed = strings.TrimSpace(trimmed)
			if len(trimmed) < minBulletLen || seen[line] {
				continue
			}

			matched := false
			for _, cs := range skills {
				if _, ok := cs.findEvidence(line); ok {
					matched = true
					break
				}
			}
			if matched {
				continue
			}

			seen[line] = true
			out = append(out, UnmatchedBullet{Text: line, Required: sec.Kind == SectionRequirements})
		}
	}

	return out
}
