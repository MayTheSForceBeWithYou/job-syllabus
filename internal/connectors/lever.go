package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// LeverConnector talks to the public, unauthenticated Lever postings API.
// See docs/design.md §5 — Lever returns structured `lists` (requirements as
// arrays), the best data quality of any Tier 1 source.
type LeverConnector struct {
	client *http.Client
}

func NewLeverConnector(client *http.Client) *LeverConnector {
	return &LeverConnector{client: client}
}

func (c *LeverConnector) Name() string { return "lever" }

// LeverList is one entry of Lever's `lists` array — typically a labeled
// section like "Requirements" or "Nice to have" with its own HTML content.
// Extraction (Phase 1, pending) should prefer these over BodyHTML when
// present since they arrive already segmented.
type LeverList struct {
	Text    string `json:"text"`
	Content string `json:"content"`
}

type leverPosting struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	HostedURL  string `json:"hostedUrl"`
	CreatedAt  int64  `json:"createdAt"` // epoch millis
	Categories struct {
		Location string `json:"location"`
	} `json:"categories"`
	Description string      `json:"description"`
	Lists       []LeverList `json:"lists"`
}

func (c *LeverConnector) Fetch(ctx context.Context, cfg CompanyConfig) ([]RawPosting, error) {
	url := fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", cfg.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("lever: build request for %s: %w", cfg.Slug, err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lever: fetch %s: %w", cfg.Slug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lever: %s returned status %d", cfg.Slug, resp.StatusCode)
	}

	var parsed []leverPosting
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("lever: decode response for %s: %w", cfg.Slug, err)
	}

	postings := make([]RawPosting, 0, len(parsed))
	for _, p := range parsed {
		postings = append(postings, RawPosting{
			ExternalID: p.ID,
			URL:        p.HostedURL,
			Title:      p.Text,
			Location:   p.Categories.Location,
			PostedAt:   time.UnixMilli(p.CreatedAt),
			BodyHTML:   p.Description,
			Structured: map[string]any{"lists": p.Lists},
		})
	}
	return postings, nil
}
