package connectors

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// maxAttempts is the initial attempt plus at most 2 retries, per operator
// instruction after the ingest CLI hung for 34+ minutes with no visibility
// into what it was doing.
const maxAttempts = 3

// FetchWithRetry calls fetch up to maxAttempts times with exponential
// backoff (500ms, 1s), logging a WARN before each retry and an ERROR with
// elapsed time when the company is given up on. It does not retry past a
// context deadline/cancellation — that's the caller's per-company timeout
// firing, not a transient failure.
func FetchWithRetry(ctx context.Context, logger *slog.Logger, companySlug string, fetch func(ctx context.Context) ([]RawPosting, error)) ([]RawPosting, error) {
	start := time.Now()
	backoff := 500 * time.Millisecond
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attemptStart := time.Now()
		postings, err := fetch(ctx)
		if err == nil {
			return postings, nil
		}
		lastErr = err

		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			logger.Error("connector fetch timed out, skipping company",
				"company", companySlug, "attempt", attempt,
				"elapsed", time.Since(start).String(), "error", err.Error())
			return nil, err
		}

		if attempt == maxAttempts {
			logger.Error("connector fetch failed after max attempts, skipping company",
				"company", companySlug, "attempts", attempt,
				"elapsed", time.Since(start).String(), "error", err.Error())
			break
		}

		logger.Warn("connector fetch failed, retrying",
			"company", companySlug, "attempt", attempt, "maxAttempts", maxAttempts,
			"attemptElapsed", time.Since(attemptStart).String(),
			"backoff", backoff.String(), "error", err.Error())

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			logger.Error("context done while backing off, skipping company",
				"company", companySlug, "elapsed", time.Since(start).String(), "error", ctx.Err())
			return nil, ctx.Err()
		}
		backoff *= 2
	}

	return nil, lastErr
}
