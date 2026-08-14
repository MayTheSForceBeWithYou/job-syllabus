// Package rawstore persists each posting's raw fetched content (HTML body
// plus ATS-specific structured data) to S3, keyed by posting ID. This is
// the queue boundary's missing half: model.Posting has no body field
// (docs/design.md §4's schema is metadata-only), so once cmd/ingest stops
// running extraction inline and instead hands the posting off to
// cmd/worker via SQS (docs/design.md §9), the worker needs somewhere
// other than the original in-memory RawPosting to get the body back from.
package rawstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/connectors"
)

// Key derives the S3 key for a posting's raw content. Exported so callers
// that only need to reference the key (not immediately write it) — e.g.
// stamping model.Posting.RawS3Key — don't have to duplicate the format.
func Key(postingID string) string {
	return "raw/" + postingID + ".json"
}

// Put uploads a posting's raw connector output as JSON and returns the S3
// key it was written to.
func Put(ctx context.Context, s3c *s3.Client, bucket, postingID string, rp connectors.RawPosting) (key string, err error) {
	key = Key(postingID)
	body, err := json.Marshal(rp)
	if err != nil {
		return "", fmt.Errorf("marshal raw posting %s: %w", postingID, err)
	}

	_, err = s3c.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return "", fmt.Errorf("put raw posting %s: %w", postingID, err)
	}
	return key, nil
}

// Get fetches and decodes a posting's raw connector output.
func Get(ctx context.Context, s3c *s3.Client, bucket, key string) (connectors.RawPosting, error) {
	out, err := s3c.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return connectors.RawPosting{}, fmt.Errorf("get raw posting %s: %w", key, err)
	}
	defer func() { _ = out.Body.Close() }()

	body, err := io.ReadAll(out.Body)
	if err != nil {
		return connectors.RawPosting{}, fmt.Errorf("read raw posting %s: %w", key, err)
	}

	var rp connectors.RawPosting
	if err := json.Unmarshal(body, &rp); err != nil {
		return connectors.RawPosting{}, fmt.Errorf("unmarshal raw posting %s: %w", key, err)
	}
	return rp, nil
}
