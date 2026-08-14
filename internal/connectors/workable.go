package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// WorkableConnector talks to the public, unauthenticated Workable widget
// API. See docs/design.md §5. Response shape confirmed against a real
// account (apply.workable.com/api/v1/widget/accounts/ripple) before
// writing this. `?details=true` is undocumented but real — without it the
// per-job list entries have no description field at all, forcing a
// second call per job; with it, the full HTML description is inlined
// into the same single-call response.
type WorkableConnector struct {
	client *http.Client
}

func NewWorkableConnector(client *http.Client) *WorkableConnector {
	return &WorkableConnector{client: client}
}

func (c *WorkableConnector) Name() string { return "workable" }

type workableResponse struct {
	Name string        `json:"name"`
	Jobs []workableJob `json:"jobs"`
}

type workableJob struct {
	Title          string `json:"title"`
	Shortcode      string `json:"shortcode"`
	URL            string `json:"url"`
	PublishedOn    string `json:"published_on"` // "YYYY-MM-DD"
	City           string `json:"city"`
	State          string `json:"state"`
	Country        string `json:"country"`
	Description    string `json:"description"` // only present with ?details=true
	EmploymentType string `json:"employment_type"`
}

func (c *WorkableConnector) Fetch(ctx context.Context, cfg CompanyConfig) ([]RawPosting, error) {
	start := time.Now()
	url := fmt.Sprintf("https://apply.workable.com/api/v1/widget/accounts/%s?details=true", cfg.Token)
	slog.Info("workable: fetching", "company", cfg.Slug, "url", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("workable: build request for %s: %w", cfg.Slug, err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		slog.Error("workable: request failed", "company", cfg.Slug, "url", url,
			"elapsed", time.Since(start).String(), "error", err.Error())
		return nil, fmt.Errorf("workable: fetch %s: %w", cfg.Slug, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Error("workable: non-200 response", "company", cfg.Slug, "url", url,
			"status", resp.StatusCode, "elapsed", time.Since(start).String())
		return nil, fmt.Errorf("workable: %s returned status %d", cfg.Slug, resp.StatusCode)
	}

	var parsed workableResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		slog.Error("workable: decode failed", "company", cfg.Slug, "url", url,
			"elapsed", time.Since(start).String(), "error", err.Error())
		return nil, fmt.Errorf("workable: decode response for %s: %w", cfg.Slug, err)
	}

	postings := make([]RawPosting, 0, len(parsed.Jobs))
	for _, j := range parsed.Jobs {
		postedAt, _ := time.Parse("2006-01-02", j.PublishedOn)
		postings = append(postings, RawPosting{
			ExternalID: j.Shortcode,
			URL:        j.URL,
			Title:      j.Title,
			Location:   joinLocation(j.City, j.State, j.Country),
			PostedAt:   postedAt,
			BodyHTML:   j.Description,
		})
	}

	slog.Info("workable: fetch complete", "company", cfg.Slug,
		"jobs", len(postings), "elapsed", time.Since(start).String())
	return postings, nil
}

func joinLocation(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += ", "
		}
		out += p
	}
	return out
}
