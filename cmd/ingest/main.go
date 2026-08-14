// cmd/ingest fetches postings from every company in data/companies.yaml via
// their registered ATS connector, extracts skills (docs/design.md §6
// Stages 1-3), and persists both to DynamoDB Local. `report` prints the
// ranked skill-frequency table — the actual DoD deliverable.
//
// Usage:
//
//	go run ./cmd/ingest ingest
//	go run ./cmd/ingest report
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/config"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/connectors"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/dedupe"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/extract"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: ingest [ingest|report]")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger) // connectors/store packages log via slog.Default()

	// Overall deadline for the whole run, so a chain of misbehaving
	// companies still terminates instead of hanging indefinitely (this is
	// exactly what happened before: no deadline anywhere, so it ran for
	// 34+ minutes in silence).
	runTimeout := envDuration("INGEST_TIMEOUT", 15*time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	endpoint := envOr("DYNAMODB_ENDPOINT", "http://localhost:8000")

	storeCtx, storeCancel := context.WithTimeout(ctx, 30*time.Second)
	s, err := store.New(storeCtx, endpoint)
	storeCancel()
	if err != nil {
		log.Fatalf("connect to store: %v", err)
	}

	tableCtx, tableCancel := context.WithTimeout(ctx, 30*time.Second)
	err = s.EnsureTable(tableCtx)
	tableCancel()
	if err != nil {
		log.Fatalf("ensure table: %v", err)
	}

	switch os.Args[1] {
	case "ingest":
		runIngest(ctx, s, logger)
	case "report":
		runReport(ctx, s)
	default:
		log.Fatalf("unknown subcommand %q (want ingest|report)", os.Args[1])
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	secs, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return time.Duration(secs) * time.Second
}

func runIngest(ctx context.Context, s *store.Store, logger *slog.Logger) {
	companiesPath := envOr("COMPANIES_FILE", "data/companies.yaml")
	skillsPath := envOr("SKILLS_FILE", "data/skills.yaml")
	companyTimeout := envDuration("COMPANY_TIMEOUT", 20*time.Second)

	httpClient := connectors.NewDefaultHTTPClient()
	registry := connectors.NewRegistry(httpClient)

	companies, err := config.LoadCompanies(companiesPath, registry)
	if err != nil {
		log.Fatalf("load %s: %v", companiesPath, err)
	}

	rawSkills, err := config.LoadSkills(skillsPath)
	if err != nil {
		log.Fatalf("load %s: %v", skillsPath, err)
	}
	skills, err := extract.CompileSkills(rawSkills)
	if err != nil {
		log.Fatalf("compile %s: %v", skillsPath, err)
	}
	logger.Info("ingest run starting", "companies", len(companies), "skills", len(skills), "companyTimeout", companyTimeout.String())

	now := time.Now().UTC()
	var totalFetched, totalMatched, totalCreated, totalUpdated, totalSkipped, totalSkillEdges int

	for i, c := range companies {
		logger.Info("company starting", "index", i+1, "of", len(companies), "company", c.Slug, "ats", c.ATS)

		conn, err := connectors.Get(registry, c.ATS)
		if err != nil {
			logger.Error("no connector registered, skipping company", "company", c.Slug, "ats", c.ATS, "error", err.Error())
			totalSkipped++
			continue
		}

		companyCtx, companyCancel := context.WithTimeout(ctx, companyTimeout)
		raw, err := connectors.FetchWithRetry(companyCtx, logger, c.Slug, func(fetchCtx context.Context) ([]connectors.RawPosting, error) {
			return conn.Fetch(fetchCtx, c)
		})
		companyCancel()
		if err != nil {
			// FetchWithRetry already logged the ERROR with elapsed time and
			// cause (timeout vs. non-200 vs. decode failure etc).
			totalSkipped++
			continue
		}
		totalFetched += len(raw)

		matched := filterByRole(raw, c.RoleFilters)
		totalMatched += len(matched)

		var created, updated, skillEdges int
		for _, rp := range matched {
			posting, err := toPosting(c, rp, now)
			if err != nil {
				logger.Warn("skipping unparseable posting", "company", c.Slug, "externalId", rp.ExternalID, "error", err.Error())
				continue
			}
			stored, wasCreated, err := s.UpsertPosting(ctx, posting)
			if err != nil {
				logger.Warn("failed to store posting", "company", c.Slug, "postingId", posting.ID, "error", err.Error())
				continue
			}
			if wasCreated {
				created++
			} else {
				updated++
			}

			n, err := extractAndStoreSkills(ctx, s, stored, rp, skills)
			if err != nil {
				logger.Warn("extraction failed", "company", c.Slug, "postingId", stored.ID, "error", err.Error())
				continue
			}
			skillEdges += n
		}
		totalCreated += created
		totalUpdated += updated
		totalSkillEdges += skillEdges

		logger.Info("company complete", "company", c.Slug,
			"fetched", len(raw), "roleMatched", len(matched), "new", created, "updated", updated, "skillEdges", skillEdges)
	}

	logger.Info("ingest run complete",
		"companies", len(companies), "skipped", totalSkipped,
		"fetched", totalFetched, "roleMatched", totalMatched,
		"new", totalCreated, "updated", totalUpdated, "skillEdges", totalSkillEdges)
}

// extractAndStoreSkills runs Stages 1-3 (normalize, segment, dictionary
// match) on a freshly-upserted posting, writes the resulting PostingSkill
// edges, and stamps ExtractVer/SkillCount onto the stored posting. Returns
// the number of skills matched.
func extractAndStoreSkills(ctx context.Context, s *store.Store, stored model.Posting, rp connectors.RawPosting, skills []extract.CompiledSkill) (int, error) {
	sections, err := extract.SegmentPosting(rp)
	if err != nil {
		return 0, fmt.Errorf("segment: %w", err)
	}

	matches := extract.MatchSkills(stored.ID, sections, skills)
	for _, m := range matches {
		if err := s.PutSkillEdge(ctx, m); err != nil {
			return 0, fmt.Errorf("store skill edge %s: %w", m.SkillID, err)
		}
	}

	stored.ExtractVer = extract.Version
	stored.SkillCount = len(matches)
	if err := s.PutPosting(ctx, stored); err != nil {
		return 0, fmt.Errorf("update posting extraction fields: %w", err)
	}
	return len(matches), nil
}

// filterByRole keeps postings whose title contains any of the company's
// role filter substrings, case-insensitively. Real ATS-side filtering
// (where supported) still happens in the connector; this is the client-side
// backstop docs/design.md §5 calls for.
func filterByRole(postings []connectors.RawPosting, filters []string) []connectors.RawPosting {
	var out []connectors.RawPosting
	for _, p := range postings {
		title := strings.ToLower(p.Title)
		for _, f := range filters {
			if strings.Contains(title, strings.ToLower(f)) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

func toPosting(c connectors.CompanyConfig, rp connectors.RawPosting, now time.Time) (model.Posting, error) {
	canonicalURL, err := dedupe.CanonicalizeURL(rp.URL)
	if err != nil {
		return model.Posting{}, fmt.Errorf("canonicalize URL %q: %w", rp.URL, err)
	}
	id := dedupe.PostingID(canonicalURL)

	postedAt := rp.PostedAt
	if postedAt.IsZero() {
		postedAt = now
	}

	// Content hash is over the raw body for now; it moves to normalized
	// text once internal/extract's Normalize stage exists (docs/design.md
	// §6 Stage 1), at which point cosmetic HTML changes won't force
	// spurious re-extraction.
	contentHash := dedupe.ContentHash(rp.BodyHTML)

	return model.Posting{
		ID:          id,
		CompanySlug: c.Slug,
		Title:       rp.Title,
		RoleFamily:  model.RoleUnclassified, // role-family classification is a later phase
		Location:    rp.Location,
		URL:         canonicalURL,
		Source:      c.ATS,
		PostedAt:    postedAt,
		LastSeenAt:  now,
		ContentHash: contentHash,
		ExtractVer:  0,
	}, nil
}

type skillRow struct {
	display    string
	category   string
	count      int
	required   int
	niceToHave int
}

func runReport(ctx context.Context, s *store.Store) {
	postings, err := s.ListAllPostings(ctx)
	if err != nil {
		log.Fatalf("list postings: %v", err)
	}
	if len(postings) == 0 {
		fmt.Println("No postings in the store yet — run `make ingest` first.")
		return
	}

	edges, err := s.ListAllSkillEdges(ctx)
	if err != nil {
		log.Fatalf("list skill edges: %v", err)
	}

	skillsPath := envOr("SKILLS_FILE", "data/skills.yaml")
	rawSkills, err := config.LoadSkills(skillsPath)
	if err != nil {
		log.Fatalf("load %s: %v", skillsPath, err)
	}
	displayByID := make(map[string]model.Skill, len(rawSkills))
	for _, sk := range rawSkills {
		displayByID[sk.ID] = sk
	}

	rows := make(map[string]*skillRow)
	for _, e := range edges {
		row, ok := rows[e.SkillID]
		if !ok {
			sk := displayByID[e.SkillID]
			row = &skillRow{display: sk.Display, category: sk.Category}
			if row.display == "" {
				row.display = e.SkillID // skills.yaml changed since this edge was written
			}
			rows[e.SkillID] = row
		}
		row.count++
		if e.Required {
			row.required++
		} else {
			row.niceToHave++
		}
	}

	ids := make([]string, 0, len(rows))
	for id := range rows {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if rows[ids[i]].count != rows[ids[j]].count {
			return rows[ids[i]].count > rows[ids[j]].count
		}
		return ids[i] < ids[j] // stable tie-break
	})

	skillWidth, categoryWidth := len("SKILL"), len("CATEGORY")
	for _, id := range ids {
		if l := len(rows[id].display); l > skillWidth {
			skillWidth = l
		}
		if l := len(rows[id].category); l > categoryWidth {
			categoryWidth = l
		}
	}

	fmt.Printf("=== Skill frequency across %d postings (%d companies) ===\n", len(postings), countCompanies(postings))
	fmt.Printf("%-*s %-*s %6s %10s %6s %12s\n", skillWidth, "SKILL", categoryWidth, "CATEGORY", "COUNT", "% OF POSTS", "REQ'D", "NICE-TO-HAVE")
	for _, id := range ids {
		r := rows[id]
		pct := float64(r.count) / float64(len(postings)) * 100
		fmt.Printf("%-*s %-*s %6d %9.1f%% %6d %12d\n", skillWidth, r.display, categoryWidth, r.category, r.count, pct, r.required, r.niceToHave)
	}

	if len(rows) == 0 {
		fmt.Println("(no skills matched — every posting's requirements/nice_to_have sections came up empty; see NEXT_STEPS.md's heading-classification note)")
	}
	fmt.Printf("\n%d distinct skills matched across %d postings\n", len(rows), len(postings))
}

func countCompanies(postings []model.Posting) int {
	seen := map[string]bool{}
	for _, p := range postings {
		seen[p.CompanySlug] = true
	}
	return len(seen)
}
