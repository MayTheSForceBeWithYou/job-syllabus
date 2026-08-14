package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// WorkdayConnector talks to Workday's undocumented-but-consistent CXS
// (career-site) JSON API. See docs/design.md §5 — "try this before
// reaching for the scraper." Response shapes for both the list and detail
// calls confirmed against a real, reachable tenant
// (citi.wd5.myworkdayjobs.com) before writing this, since Workday
// publishes no formal API docs and the URL shape genuinely varies by
// tenant.
//
// Workday needs two pieces of per-company identity beyond the usual
// single Token — the subdomain (e.g. "citi.wd5", varying per tenant's
// Workday pod) and the career "site" (e.g. "2" or "External", an
// opaque per-tenant identifier with no discoverable pattern; find it by
// inspecting the company's own careers page network requests). Both are
// packed into Token as "{subdomain}/{site}" rather than widening
// CompanyConfig, since no other connector needs more than one field.
type WorkdayConnector struct {
	client *http.Client
}

func NewWorkdayConnector(client *http.Client) *WorkdayConnector {
	return &WorkdayConnector{client: client}
}

func (c *WorkdayConnector) Name() string { return "workday" }

type workdayListRequest struct {
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	SearchText string `json:"searchText"`
}

type workdayListResponse struct {
	Total       int                  `json:"total"`
	JobPostings []workdayListPosting `json:"jobPostings"`
}

type workdayListPosting struct {
	Title        string `json:"title"`
	ExternalPath string `json:"externalPath"`
}

type workdayDetailResponse struct {
	JobPostingInfo struct {
		Title          string `json:"title"`
		JobDescription string `json:"jobDescription"`
		Location       string `json:"location"`
		StartDate      string `json:"startDate"` // "YYYY-MM-DD"
		JobReqID       string `json:"jobReqId"`
		ExternalURL    string `json:"externalUrl"`
	} `json:"jobPostingInfo"`
}

// workdayPageSize / workdayDetailConcurrency: Workday's list endpoint is
// paginated (no single-call "give me everything"), and — like
// SmartRecruiters — gives no description in the list itself, so every
// posting needs its own detail call. 20 is Workday's own server-side
// max — confirmed against a real tenant; anything above it is rejected
// outright with a bare "HTTP_400" (no explanatory message).
const (
	workdayPageSize          = 20
	workdayDetailConcurrency = 5
)

func (c *WorkdayConnector) Fetch(ctx context.Context, cfg CompanyConfig) ([]RawPosting, error) {
	start := time.Now()

	subdomain, site, err := splitWorkdayToken(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("workday: %s: %w", cfg.Slug, err)
	}
	tenant := strings.SplitN(subdomain, ".", 2)[0]
	base := fmt.Sprintf("https://%s.myworkdayjobs.com/wday/cxs/%s/%s", subdomain, tenant, site)
	slog.Info("workday: fetching", "company", cfg.Slug, "base", base)

	all, err := c.fetchAllListings(ctx, cfg, base)
	if err != nil {
		return nil, err
	}

	postings := make([]RawPosting, len(all))
	errs := make([]error, len(all))
	sem := make(chan struct{}, workdayDetailConcurrency)
	var wg sync.WaitGroup
	for i, item := range all {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, item workdayListPosting) {
			defer wg.Done()
			defer func() { <-sem }()
			p, err := c.fetchDetail(ctx, base, item)
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
			slog.Warn("workday: skipping posting after detail fetch failure",
				"company", cfg.Slug, "path", all[i].ExternalPath, "error", errs[i].Error())
			continue
		}
		out = append(out, p)
	}

	slog.Info("workday: fetch complete", "company", cfg.Slug,
		"jobs", len(out), "failed", failed, "elapsed", time.Since(start).String())
	return out, nil
}

// splitWorkdayToken parses "{subdomain}/{site}" — e.g. "citi.wd5/2".
func splitWorkdayToken(token string) (subdomain, site string, err error) {
	parts := strings.SplitN(token, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("token %q must be \"{subdomain}/{site}\" (e.g. \"citi.wd5/2\")", token)
	}
	return parts[0], parts[1], nil
}

func (c *WorkdayConnector) fetchAllListings(ctx context.Context, cfg CompanyConfig, base string) ([]workdayListPosting, error) {
	var all []workdayListPosting
	offset := 0
	for {
		body, err := json.Marshal(workdayListRequest{Limit: workdayPageSize, Offset: offset, SearchText: ""})
		if err != nil {
			return nil, fmt.Errorf("workday: encode list request for %s: %w", cfg.Slug, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/jobs", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("workday: build list request for %s: %w", cfg.Slug, err)
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("workday: fetch list %s (offset %d): %w", cfg.Slug, offset, err)
		}
		var parsed workdayListResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&parsed)
		status := resp.StatusCode
		_ = resp.Body.Close()

		if status != http.StatusOK {
			return nil, fmt.Errorf("workday: %s list (offset %d) returned status %d", cfg.Slug, offset, status)
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("workday: decode list for %s (offset %d): %w", cfg.Slug, offset, decodeErr)
		}

		all = append(all, parsed.JobPostings...)
		offset += workdayPageSize
		if offset >= parsed.Total || len(parsed.JobPostings) == 0 {
			break
		}
	}
	return all, nil
}

func (c *WorkdayConnector) fetchDetail(ctx context.Context, base string, item workdayListPosting) (RawPosting, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+item.ExternalPath, nil)
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

	var detail workdayDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return RawPosting{}, fmt.Errorf("decode detail: %w", err)
	}

	postedAt, _ := time.Parse("2006-01-02", detail.JobPostingInfo.StartDate)
	url := detail.JobPostingInfo.ExternalURL
	if url == "" {
		url = base + item.ExternalPath
	}
	return RawPosting{
		ExternalID: detail.JobPostingInfo.JobReqID,
		URL:        url,
		Title:      detail.JobPostingInfo.Title,
		Location:   detail.JobPostingInfo.Location,
		PostedAt:   postedAt,
		BodyHTML:   detail.JobPostingInfo.JobDescription,
	}, nil
}
