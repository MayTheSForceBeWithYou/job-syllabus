// Package model defines the domain types persisted in the single-table
// DynamoDB design described in docs/design.md §4.
package model

import "time"

// RoleFamily buckets a posting by the kind of engineering work it describes.
type RoleFamily string

const (
	RoleBuildRelease   RoleFamily = "build_release"
	RoleLiveOpsBackend RoleFamily = "liveops_backend"
	RolePlatformInfra  RoleFamily = "platform_infra"
	RoleGameplay       RoleFamily = "gameplay"
	RoleTools          RoleFamily = "tools"
	RoleSecurity       RoleFamily = "security"
	RoleUnclassified   RoleFamily = "unclassified"
	RoleOther          RoleFamily = "other"
)

// CompanyTier mirrors the `tier` enum in data/companies.yaml.
type CompanyTier string

const (
	TierPlatform CompanyTier = "platform"
	TierAAA      CompanyTier = "aaa"
	TierMidsize  CompanyTier = "midsize"
	TierIndie    CompanyTier = "indie"
	TierTools    CompanyTier = "tools"
)

// Company is the `COMPANY#<slug>` / `META` item.
type Company struct {
	Slug         string      `dynamodbav:"slug"`
	Name         string      `dynamodbav:"name"`
	Tier         CompanyTier `dynamodbav:"tier"`
	ATS          string      `dynamodbav:"ats"`
	LastIngestAt *time.Time  `dynamodbav:"lastIngestAt,omitempty"`
}

// Posting is the `POSTING#<id>` / `META` item. Field order and comments match
// docs/design.md §4.
type Posting struct {
	ID          string     `dynamodbav:"id"`
	CompanySlug string     `dynamodbav:"companySlug"`
	Title       string     `dynamodbav:"title"`
	RoleFamily  RoleFamily `dynamodbav:"roleFamily"`
	Location    string     `dynamodbav:"location"`
	Remote      bool       `dynamodbav:"remote"`
	URL         string     `dynamodbav:"url"`
	Source      string     `dynamodbav:"source"` // greenhouse | lever | ashby | workday | scraper | manual
	PostedAt    time.Time  `dynamodbav:"postedAt"`
	FirstSeenAt time.Time  `dynamodbav:"firstSeenAt"`
	LastSeenAt  time.Time  `dynamodbav:"lastSeenAt"`
	ClosedAt    *time.Time `dynamodbav:"closedAt,omitempty"`
	RawS3Key    string     `dynamodbav:"rawS3Key,omitempty"` // empty until Phase 2+ writes to real S3
	ContentHash string     `dynamodbav:"contentHash"`
	ExtractVer  int        `dynamodbav:"extractVer"`
	SkillCount  int        `dynamodbav:"skillCount"`
}

// PostingSkill is the `POSTING#<id>` / `SKILL#<skillId>` edge item.
type PostingSkill struct {
	PostingID  string  `dynamodbav:"postingId"`
	SkillID    string  `dynamodbav:"skillId"`
	Required   bool    `dynamodbav:"required"`
	Evidence   string  `dynamodbav:"evidence"` // <=200 chars, matched span
	Confidence float32 `dynamodbav:"confidence"`
	Method     string  `dynamodbav:"method"` // dict | llm
}

// Skill is the canonical `SKILL#<skillId>` / `META` item, sourced from
// data/skills.yaml.
type Skill struct {
	ID         string   `yaml:"id" dynamodbav:"id"`
	Display    string   `yaml:"display" dynamodbav:"display"`
	Category   string   `yaml:"category" dynamodbav:"category"`
	Aliases    []string `yaml:"aliases" dynamodbav:"aliases"`
	Patterns   []string `yaml:"patterns,omitempty" dynamodbav:"patterns,omitempty"`
	Deprecated bool     `yaml:"deprecated,omitempty" dynamodbav:"deprecated,omitempty"`
	MergedInto string   `yaml:"mergedInto,omitempty" dynamodbav:"mergedInto,omitempty"`
}
