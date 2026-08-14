package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// AshbyConnector talks to the public, unauthenticated Ashby job board API.
// See docs/design.md §5. Response shape confirmed against a real board
// (api.ashbyhq.com/posting-api/job-board/linear) before writing this —
// Ashby's docs don't publish a formal schema.
type AshbyConnector struct {
	client *http.Client
}

func NewAshbyConnector(client *http.Client) *AshbyConnector {
	return &AshbyConnector{client: client}
}

func (c *AshbyConnector) Name() string { return "ashby" }

type ashbyResponse struct {
	Jobs []ashbyJob `json:"jobs"`
}

type ashbyJob struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Location        string `json:"location"`
	PublishedAt     string `json:"publishedAt"`
	JobURL          string `json:"jobUrl"`
	DescriptionHTML string `json:"descriptionHtml"`
}

func (c *AshbyConnector) Fetch(ctx context.Context, cfg CompanyConfig) ([]RawPosting, error) {
	start := time.Now()
	url := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", cfg.Token)
	slog.Info("ashby: fetching", "company", cfg.Slug, "url", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("ashby: build request for %s: %w", cfg.Slug, err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		slog.Error("ashby: request failed", "company", cfg.Slug, "url", url,
			"elapsed", time.Since(start).String(), "error", err.Error())
		return nil, fmt.Errorf("ashby: fetch %s: %w", cfg.Slug, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Error("ashby: non-200 response", "company", cfg.Slug, "url", url,
			"status", resp.StatusCode, "elapsed", time.Since(start).String())
		return nil, fmt.Errorf("ashby: %s returned status %d", cfg.Slug, resp.StatusCode)
	}

	var parsed ashbyResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		slog.Error("ashby: decode failed", "company", cfg.Slug, "url", url,
			"elapsed", time.Since(start).String(), "error", err.Error())
		return nil, fmt.Errorf("ashby: decode response for %s: %w", cfg.Slug, err)
	}

	postings := make([]RawPosting, 0, len(parsed.Jobs))
	for _, j := range parsed.Jobs {
		postedAt, _ := time.Parse(time.RFC3339, j.PublishedAt)
		postings = append(postings, RawPosting{
			ExternalID: j.ID,
			URL:        j.JobURL,
			Title:      j.Title,
			Location:   j.Location,
			PostedAt:   postedAt,
			BodyHTML:   j.DescriptionHTML,
		})
	}

	slog.Info("ashby: fetch complete", "company", cfg.Slug,
		"jobs", len(postings), "elapsed", time.Since(start).String())
	return postings, nil
}
