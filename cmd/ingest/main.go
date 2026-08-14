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
	"strconv"
	"strings"
	"time"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/config"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/connectors"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/dedupe"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/extract"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/rank"
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
	var totalFetched, totalMatched, totalCreated, totalUpdated, totalClosed, totalSkipped, totalSkillEdges int

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

			// Only a genuinely new posting ID needs the cross-post check —
			// an already-known posting was claimed under this same content
			// hash the first time it was created, so re-checking it on
			// every re-ingest would find its own marker and misfire.
			if wasCreated {
				firstSeen, err := s.ClaimContentHash(ctx, stored.ContentHash)
				if err != nil {
					logger.Warn("dedup content-hash check failed", "company", c.Slug, "postingId", stored.ID, "error", err.Error())
				} else if !firstSeen {
					// Same job, cross-posted under a different URL — collapse
					// to whichever posting claimed this content first
					// (docs/design.md §5) by discarding this one outright.
					if err := s.DeletePosting(ctx, stored.ID); err != nil {
						logger.Warn("failed to remove cross-posted duplicate", "company", c.Slug, "postingId", stored.ID, "error", err.Error())
					}
					logger.Info("skipping cross-posted duplicate", "company", c.Slug, "postingId", stored.ID, "title", stored.Title)
					continue
				}
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

		closed, err := closeStalePostings(ctx, s, c.Slug, raw, now)
		if err != nil {
			logger.Warn("posting lifecycle check failed", "company", c.Slug, "error", err.Error())
		}
		totalClosed += closed

		logger.Info("company complete", "company", c.Slug,
			"fetched", len(raw), "roleMatched", len(matched), "new", created, "updated", updated, "closed", closed, "skillEdges", skillEdges)
	}

	logger.Info("ingest run complete",
		"companies", len(companies), "skipped", totalSkipped,
		"fetched", totalFetched, "roleMatched", totalMatched,
		"new", totalCreated, "updated", totalUpdated, "closed", totalClosed, "skillEdges", totalSkillEdges)
}

// closeStalePostings implements docs/design.md §4's lifecycle rule: a
// posting this company previously had open, but which no longer appears
// anywhere in this run's raw fetch (role-matched or not — a title/role
// change isn't "closed"), has left the ATS feed and is marked closed.
// Compares against the full raw fetch, not just the role-matched subset,
// so postings never stored (didn't match roleFilters) can't spuriously
// "close" postings that are still genuinely live at the company.
func closeStalePostings(ctx context.Context, s *store.Store, companySlug string, raw []connectors.RawPosting, now time.Time) (int, error) {
	current := make(map[string]bool, len(raw))
	for _, rp := range raw {
		canonicalURL, err := dedupe.CanonicalizeURL(rp.URL)
		if err != nil {
			continue // unparseable URL never became a stored posting either
		}
		current[dedupe.PostingID(canonicalURL)] = true
	}

	known, err := s.ListPostingsByCompany(ctx, companySlug)
	if err != nil {
		return 0, fmt.Errorf("list known postings: %w", err)
	}

	closed := 0
	for _, p := range known {
		if p.ClosedAt != nil || current[p.ID] {
			continue
		}
		if err := s.ClosePosting(ctx, p.ID, now); err != nil {
			return closed, fmt.Errorf("close posting %s: %w", p.ID, err)
		}
		closed++
	}
	return closed, nil
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

func runReport(ctx context.Context, s *store.Store) {
	allPostings, err := s.ListAllPostings(ctx)
	if err != nil {
		log.Fatalf("list postings: %v", err)
	}
	if len(allPostings) == 0 {
		fmt.Println("No postings in the store yet — run `make ingest` first.")
		return
	}

	// Active only (docs/design.md §4): a closed posting stays in the store
	// for historical trend but drops out of this ranked table, matching
	// GET /v1/skills and /v1/stats/overview's same definition of "active."
	active := make(map[string]bool, len(allPostings))
	postings := allPostings[:0:0]
	for _, p := range allPostings {
		if p.ClosedAt != nil {
			continue
		}
		active[p.ID] = true
		postings = append(postings, p)
	}
	if len(postings) == 0 {
		fmt.Println("No active postings in the store — everything ingested so far has closed.")
		return
	}

	allEdges, err := s.ListAllSkillEdges(ctx)
	if err != nil {
		log.Fatalf("list skill edges: %v", err)
	}
	edges := allEdges[:0:0]
	for _, e := range allEdges {
		if active[e.PostingID] {
			edges = append(edges, e)
		}
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

	rows := rank.Skills(edges, displayByID)

	skillWidth, categoryWidth := len("SKILL"), len("CATEGORY")
	for _, r := range rows {
		if l := len(r.Display); l > skillWidth {
			skillWidth = l
		}
		if l := len(r.Category); l > categoryWidth {
			categoryWidth = l
		}
	}

	fmt.Printf("=== Skill frequency across %d postings (%d companies) ===\n", len(postings), rank.CountCompanies(postings))
	fmt.Printf("%-*s %-*s %6s %10s %6s %12s\n", skillWidth, "SKILL", categoryWidth, "CATEGORY", "COUNT", "% OF POSTS", "REQ'D", "NICE-TO-HAVE")
	for _, r := range rows {
		pct := float64(r.Count) / float64(len(postings)) * 100
		fmt.Printf("%-*s %-*s %6d %9.1f%% %6d %12d\n", skillWidth, r.Display, categoryWidth, r.Category, r.Count, pct, r.Required, r.NiceToHave)
	}

	if len(rows) == 0 {
		fmt.Println("(no skills matched — every posting's requirements/nice_to_have sections came up empty; see NEXT_STEPS.md's heading-classification note)")
	}
	fmt.Printf("\n%d distinct skills matched across %d postings\n", len(rows), len(postings))
}
