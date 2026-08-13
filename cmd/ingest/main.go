// cmd/ingest fetches postings from every company in data/companies.yaml via
// their registered ATS connector and persists them to DynamoDB Local. It
// also carries the (currently posting-count-only) `report` subcommand,
// pending the extraction pipeline from docs/design.md §6.
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
	"os"
	"sort"
	"strings"
	"time"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/config"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/connectors"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/dedupe"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: ingest [ingest|report]")
	}

	ctx := context.Background()
	endpoint := envOr("DYNAMODB_ENDPOINT", "http://localhost:8000")

	s, err := store.New(ctx, endpoint)
	if err != nil {
		log.Fatalf("connect to store: %v", err)
	}
	if err := s.EnsureTable(ctx); err != nil {
		log.Fatalf("ensure table: %v", err)
	}

	switch os.Args[1] {
	case "ingest":
		runIngest(ctx, s)
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

func runIngest(ctx context.Context, s *store.Store) {
	companiesPath := envOr("COMPANIES_FILE", "data/companies.yaml")

	httpClient := connectors.NewDefaultHTTPClient()
	registry := connectors.NewRegistry(httpClient)

	companies, err := config.LoadCompanies(companiesPath, registry)
	if err != nil {
		log.Fatalf("load %s: %v", companiesPath, err)
	}

	now := time.Now().UTC()
	var totalFetched, totalMatched, totalCreated, totalUpdated int

	for _, c := range companies {
		conn, err := connectors.Get(registry, c.ATS)
		if err != nil {
			log.Printf("%s: %v", c.Slug, err)
			continue
		}

		raw, err := conn.Fetch(ctx, c)
		if err != nil {
			log.Printf("%s: fetch failed: %v", c.Slug, err)
			continue
		}
		totalFetched += len(raw)

		matched := filterByRole(raw, c.RoleFilters)
		totalMatched += len(matched)

		var created, updated int
		for _, rp := range matched {
			posting, err := toPosting(c, rp, now)
			if err != nil {
				log.Printf("%s: skip posting %s: %v", c.Slug, rp.ExternalID, err)
				continue
			}
			wasCreated, err := s.UpsertPosting(ctx, posting)
			if err != nil {
				log.Printf("%s: store posting %s: %v", c.Slug, posting.ID, err)
				continue
			}
			if wasCreated {
				created++
			} else {
				updated++
			}
		}
		totalCreated += created
		totalUpdated += updated

		fmt.Printf("%-20s fetched=%-4d role-matched=%-4d new=%-4d updated=%-4d\n",
			c.Slug, len(raw), len(matched), created, updated)
	}

	fmt.Printf("\ncompanies=%d fetched=%d role-matched=%d new=%d updated=%d\n",
		len(companies), totalFetched, totalMatched, totalCreated, totalUpdated)
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
	postings, err := s.ListAllPostings(ctx)
	if err != nil {
		log.Fatalf("list postings: %v", err)
	}

	if len(postings) == 0 {
		fmt.Println("No postings in the store yet — run `make ingest` first.")
		return
	}

	byCompany := map[string]int{}
	for _, p := range postings {
		byCompany[p.CompanySlug]++
	}

	companies := make([]string, 0, len(byCompany))
	for c := range byCompany {
		companies = append(companies, c)
	}
	sort.Slice(companies, func(i, j int) bool { return byCompany[companies[i]] > byCompany[companies[j]] })

	fmt.Println("=== Posting counts by company (interim — skill ranking pending extraction pipeline) ===")
	for _, c := range companies {
		fmt.Printf("%-24s %d\n", c, byCompany[c])
	}
	fmt.Printf("\ntotal postings: %d across %d companies\n", len(postings), len(companies))
	fmt.Println("\nNOTE: the ranked skill-frequency table (the actual DoD deliverable) requires")
	fmt.Println("the extraction pipeline (docs/design.md §6), which is pending the segmentation")
	fmt.Println("heuristics checkpoint. This report will be replaced once that lands.")
}
