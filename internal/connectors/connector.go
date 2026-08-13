// Package connectors implements per-ATS fetchers for job postings, per
// docs/design.md §5 (Tier 1 — ATS public JSON APIs).
package connectors

import (
	"context"
	"time"
)

// Connector fetches raw postings from one ATS for one company.
type Connector interface {
	// Name identifies the ATS this connector talks to, e.g. "greenhouse", "lever".
	// Must match the `ats` field in companies.yaml.
	Name() string

	// Fetch retrieves all currently-listed postings for the given company.
	// Implementations do their own role filtering using cfg.RoleFilters where
	// the ATS supports server-side filtering; otherwise the caller filters
	// client-side using the same list.
	Fetch(ctx context.Context, cfg CompanyConfig) ([]RawPosting, error)
}

// CompanyConfig is one entry from data/companies.yaml.
type CompanyConfig struct {
	Slug        string   `yaml:"slug"`
	Name        string   `yaml:"name"`
	Tier        string   `yaml:"tier"` // platform | aaa | midsize | indie | tools
	ATS         string   `yaml:"ats"`  // greenhouse | lever | ashby | smartrecruiters | workable | workday
	Token       string   `yaml:"token"`
	RoleFilters []string `yaml:"roleFilters"`
}

// RawPosting is a connector's output before normalization/extraction.
type RawPosting struct {
	ExternalID string
	URL        string
	Title      string
	Location   string
	PostedAt   time.Time
	BodyHTML   string
	Structured map[string]any // ATS-specific structured data (e.g. Lever's `lists`)
}
