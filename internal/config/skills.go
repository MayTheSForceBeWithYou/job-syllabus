package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
)

var validSkillID = regexp.MustCompile(`^[a-z0-9-]+$`)

// LoadSkills reads and validates data/skills.yaml: unique id, non-empty
// category, and at least one of aliases/patterns (a skill with neither
// could never match anything, which is always a mistake in this file).
func LoadSkills(path string) ([]model.Skill, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var skills []model.Skill
	if err := yaml.Unmarshal(raw, &skills); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	seen := make(map[string]bool, len(skills))
	for _, s := range skills {
		if !validSkillID.MatchString(s.ID) {
			return nil, fmt.Errorf("skill %q: id must match %s", s.ID, validSkillID.String())
		}
		if seen[s.ID] {
			return nil, fmt.Errorf("skill %q: duplicate id", s.ID)
		}
		seen[s.ID] = true

		if s.Category == "" {
			return nil, fmt.Errorf("skill %q: category is required", s.ID)
		}
		if len(s.Aliases) == 0 && len(s.Patterns) == 0 {
			return nil, fmt.Errorf("skill %q: must have at least one alias or pattern", s.ID)
		}
	}
	return skills, nil
}
