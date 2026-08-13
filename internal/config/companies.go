// Package config loads and validates the YAML data files under data/.
package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/connectors"
)

var validSlug = regexp.MustCompile(`^[a-z0-9-]+$`)

var validTiers = map[string]bool{
	"platform": true,
	"aaa":      true,
	"midsize":  true,
	"indie":    true,
	"tools":    true,
}

// LoadCompanies reads and validates data/companies.yaml. Validation fails
// fast at load time rather than per-company at fetch time, per the schema
// review agreed in the Phase 1 checkpoint.
func LoadCompanies(path string, registry map[string]connectors.Connector) ([]connectors.CompanyConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var companies []connectors.CompanyConfig
	if err := yaml.Unmarshal(raw, &companies); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	seen := make(map[string]bool, len(companies))
	for _, c := range companies {
		if !validSlug.MatchString(c.Slug) {
			return nil, fmt.Errorf("company %q: slug must match %s", c.Slug, validSlug.String())
		}
		if seen[c.Slug] {
			return nil, fmt.Errorf("company %q: duplicate slug", c.Slug)
		}
		seen[c.Slug] = true

		if !validTiers[c.Tier] {
			return nil, fmt.Errorf("company %q: invalid tier %q", c.Slug, c.Tier)
		}
		if _, err := connectors.Get(registry, c.ATS); err != nil {
			return nil, fmt.Errorf("company %q: %w", c.Slug, err)
		}
		if len(c.RoleFilters) == 0 {
			return nil, fmt.Errorf("company %q: roleFilters must be non-empty", c.Slug)
		}
	}
	return companies, nil
}
