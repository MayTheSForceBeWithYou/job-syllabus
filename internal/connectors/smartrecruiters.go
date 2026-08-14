package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// SmartRecruitersConnector talks to the public, unauthenticated
// SmartRecruiters postings API. See docs/design.md §5 — genuinely a
// two-call ATS: the list endpoint returns no description at all, only
// enough to build a detail URL per posting. Response shapes for both
// calls confirmed against a real company (api.smartrecruiters.com/v1/
// companies/visa/postings) before writing this.
type SmartRecruitersConnector struct {
	client *http.Client
}

func NewSmartRecruitersConnector(client *http.Client) *SmartRecruitersConnector {
	return &SmartRecruitersConnector{client: client}
}

func (c *SmartRecruitersConnector) Name() string { return "smartrecruiters" }

type smartRecruitersListResponse struct {
	TotalFound int                       `json:"totalFound"`
	Content    []smartRecruitersListItem `json:"content"`
}

type smartRecruitersListItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ReleasedDate string `json:"releasedDate"` // RFC3339
	Location     struct {
		FullLocation string `json:"fullLocation"`
	} `json:"location"`
}

type smartRecruitersDetail struct {
	PostingURL string `json:"postingUrl"`
	JobAd      struct {
		Sections map[string]struct {
			Title string `json:"title"`
			Text  string `json:"text"`
		} `json:"sections"`
	} `json:"jobAd"`
}

// smartRecruitersDetailConcurrency bounds how many per-posting detail
// calls run at once — a company can have hundreds of open postings, and
// the list endpoint gives no way to filter server-side before fetching
// each one's body.
const smartRecruitersDetailConcurrency = 5

func (c *SmartRecruitersConnector) Fetch(ctx context.Context, cfg CompanyConfig) ([]RawPosting, error) {
	start := time.Now()
	listURL := fmt.Sprintf("https://api.smartrecruiters.com/v1/companies/%s/postings", cfg.Token)
	slog.Info("smartrecruiters: fetching", "company", cfg.Slug, "url", listURL)

	list, err := c.fetchList(ctx, cfg, listURL)
	if err != nil {
		return nil, err
	}

	postings := make([]RawPosting, len(list.Content))
	errs := make([]error, len(list.Content))
	sem := make(chan struct{}, smartRecruitersDetailConcurrency)
	var wg sync.WaitGroup
	for i, item := range list.Content {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, item smartRecruitersListItem) {
			defer wg.Done()
			defer func() { <-sem }()
			p, err := c.fetchDetail(ctx, cfg, item)
			if err != nil {
				errs[i] = err
				return
			}
			postings[i] = p
		}(i, item)
	}
	wg.Wait()

	out := make([]RawPosting, 0, len(postings))
	failed := 0
	for i, p := range postings {
		if errs[i] != nil {
			failed++
			slog.Warn("smartrecruiters: skipping posting after detail fetch failure",
				"company", cfg.Slug, "postingId", list.Content[i].ID, "error", errs[i].Error())
			continue
		}
		out = append(out, p)
	}

	slog.Info("smartrecruiters: fetch complete", "company", cfg.Slug,
		"jobs", len(out), "failed", failed, "elapsed", time.Since(start).String())
	return out, nil
}

func (c *SmartRecruitersConnector) fetchList(ctx context.Context, cfg CompanyConfig, url string) (*smartRecruitersListResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("smartrecruiters: build list request for %s: %w", cfg.Slug, err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("smartrecruiters: fetch list %s: %w", cfg.Slug, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("smartrecruiters: %s list returned status %d", cfg.Slug, resp.StatusCode)
	}

	var parsed smartRecruitersListResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("smartrecruiters: decode list for %s: %w", cfg.Slug, err)
	}
	return &parsed, nil
}

func (c *SmartRecruitersConnector) fetchDetail(ctx context.Context, cfg CompanyConfig, item smartRecruitersListItem) (RawPosting, error) {
	url := fmt.Sprintf("https://api.smartrecruiters.com/v1/companies/%s/postings/%s", cfg.Token, item.ID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return RawPosting{}, fmt.Errorf("build detail request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return RawPosting{}, fmt.Errorf("fetch detail: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return RawPosting{}, fmt.Errorf("detail returned status %d", resp.StatusCode)
	}

	var detail smartRecruitersDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return RawPosting{}, fmt.Errorf("decode detail: %w", err)
	}

	// SmartRecruiters pre-segments the body into named sections
	// (companyDescription, jobDescription, qualifications,
	// additionalInformation) — concatenated with heading markers so
	// internal/extract's Stage 2 can still classify them the same way
	// it does an unstructured Greenhouse-style body.
	body := ""
	for _, key := range []string{"jobDescription", "qualifications", "additionalInformation"} {
		section, ok := detail.JobAd.Sections[key]
		if !ok || section.Text == "" {
			continue
		}
		heading := section.Title
		if heading == "" {
			heading = key
		}
		body += "<h3>" + heading + "</h3>" + section.Text
	}

	postedAt, _ := time.Parse(time.RFC3339, item.ReleasedDate)
	return RawPosting{
		ExternalID: item.ID,
		URL:        detail.PostingURL,
		Title:      item.Name,
		Location:   item.Location.FullLocation,
		PostedAt:   postedAt,
		BodyHTML:   body,
	}, nil
}
