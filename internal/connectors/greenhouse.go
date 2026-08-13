package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// GreenhouseConnector talks to the public, unauthenticated Greenhouse Job
// Board API. See docs/design.md §5.
type GreenhouseConnector struct {
	client *http.Client
}

func NewGreenhouseConnector(client *http.Client) *GreenhouseConnector {
	return &GreenhouseConnector{client: client}
}

func (c *GreenhouseConnector) Name() string { return "greenhouse" }

type greenhouseResponse struct {
	Jobs []greenhouseJob `json:"jobs"`
}

type greenhouseJob struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	UpdatedAt   string `json:"updated_at"`
	AbsoluteURL string `json:"absolute_url"`
	Content     string `json:"content"`
	Location    struct {
		Name string `json:"name"`
	} `json:"location"`
}

func (c *GreenhouseConnector) Fetch(ctx context.Context, cfg CompanyConfig) ([]RawPosting, error) {
	start := time.Now()
	url := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true", cfg.Token)
	slog.Info("greenhouse: fetching", "company", cfg.Slug, "url", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("greenhouse: build request for %s: %w", cfg.Slug, err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		slog.Error("greenhouse: request failed", "company", cfg.Slug, "url", url,
			"elapsed", time.Since(start).String(), "error", err.Error())
		return nil, fmt.Errorf("greenhouse: fetch %s: %w", cfg.Slug, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Error("greenhouse: non-200 response", "company", cfg.Slug, "url", url,
			"status", resp.StatusCode, "elapsed", time.Since(start).String())
		return nil, fmt.Errorf("greenhouse: %s returned status %d", cfg.Slug, resp.StatusCode)
	}

	var parsed greenhouseResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		slog.Error("greenhouse: decode failed", "company", cfg.Slug, "url", url,
			"elapsed", time.Since(start).String(), "error", err.Error())
		return nil, fmt.Errorf("greenhouse: decode response for %s: %w", cfg.Slug, err)
	}

	postings := make([]RawPosting, 0, len(parsed.Jobs))
	for _, j := range parsed.Jobs {
		postedAt, _ := time.Parse(time.RFC3339, j.UpdatedAt)
		postings = append(postings, RawPosting{
			ExternalID: fmt.Sprintf("%d", j.ID),
			URL:        j.AbsoluteURL,
			Title:      j.Title,
			Location:   j.Location.Name,
			PostedAt:   postedAt,
			BodyHTML:   j.Content,
		})
	}

	slog.Info("greenhouse: fetch complete", "company", cfg.Slug,
		"jobs", len(postings), "elapsed", time.Since(start).String())
	return postings, nil
}
