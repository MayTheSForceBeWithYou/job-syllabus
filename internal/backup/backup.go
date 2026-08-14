// Package backup implements the export/import safety net for tearing down
// the dev-data Terraform stack entirely (docs/design.md §9 amendment):
// DynamoDB and S3 are already near-free at rest, so the stack is meant to
// stay running, but if the operator ever does destroy it, this is how the
// data comes back.
package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// LatestKey is always overwritten by Export and always what Import reads
// by default, so "restore the most recent backup" never requires knowing
// a timestamp.
const LatestKey = "backups/latest.jsonl.gz"

// Export scans every item in the table (any entity type — this is a
// whole-table snapshot, not scoped to postings or skills specifically) and
// uploads a gzipped JSON-lines dump to S3 at both a timestamped key and
// LatestKey. Item attribute values are decoded to generic Go values
// (attributevalue.UnmarshalMap) rather than typed structs, so the backup
// stays lossless even for entity types the current code doesn't have a
// struct for yet.
func Export(ctx context.Context, ddb *dynamodb.Client, s3c *s3.Client, table, bucket string) (itemCount int, timestampedKey string, err error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	enc := json.NewEncoder(gz)

	var lastKey map[string]types.AttributeValue
	for {
		out, scanErr := ddb.Scan(ctx, &dynamodb.ScanInput{
			TableName:         aws.String(table),
			ExclusiveStartKey: lastKey,
		})
		if scanErr != nil {
			return 0, "", fmt.Errorf("scan: %w", scanErr)
		}
		for _, item := range out.Items {
			var generic map[string]any
			if err := attributevalue.UnmarshalMap(item, &generic); err != nil {
				return 0, "", fmt.Errorf("unmarshal item: %w", err)
			}
			if err := enc.Encode(generic); err != nil {
				return 0, "", fmt.Errorf("encode item: %w", err)
			}
			itemCount++
		}
		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}

	if err := gz.Close(); err != nil {
		return 0, "", fmt.Errorf("close gzip writer: %w", err)
	}
	data := buf.Bytes()

	timestampedKey = fmt.Sprintf("backups/%s.jsonl.gz", time.Now().UTC().Format("20060102T150405Z"))
	for _, key := range []string{timestampedKey, LatestKey} {
		_, err := s3c.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(bucket),
			Key:         aws.String(key),
			Body:        bytes.NewReader(data),
			ContentType: aws.String("application/x-ndjson"),
		})
		if err != nil {
			return itemCount, "", fmt.Errorf("upload %s: %w", key, err)
		}
	}

	return itemCount, timestampedKey, nil
}

// Import downloads the given S3 key (LatestKey if empty) and restores
// every item into the table via BatchWriteItem, chunked to DynamoDB's
// 25-item batch limit. A PutRequest for an item that already exists
// overwrites it — Import is meant to run against a freshly (re)created,
// empty table, not to merge into a live one.
func Import(ctx context.Context, ddb *dynamodb.Client, s3c *s3.Client, table, bucket, key string) (itemCount int, err error) {
	if key == "" {
		key = LatestKey
	}

	obj, err := s3c.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return 0, fmt.Errorf("download %s: %w", key, err)
	}
	defer func() { _ = obj.Body.Close() }()

	gz, err := gzip.NewReader(obj.Body)
	if err != nil {
		return 0, fmt.Errorf("open gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()

	dec := json.NewDecoder(gz)
	var batch []types.WriteRequest

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, err := ddb.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{table: batch},
		})
		batch = batch[:0]
		return err
	}

	for {
		var generic map[string]any
		if decErr := dec.Decode(&generic); decErr != nil {
			if decErr == io.EOF {
				break
			}
			return itemCount, fmt.Errorf("decode item: %w", decErr)
		}
		item, avErr := attributevalue.MarshalMap(generic)
		if avErr != nil {
			return itemCount, fmt.Errorf("marshal item: %w", avErr)
		}
		batch = append(batch, types.WriteRequest{PutRequest: &types.PutRequest{Item: item}})
		itemCount++
		if len(batch) == 25 {
			if err := flush(); err != nil {
				return itemCount, fmt.Errorf("batch write: %w", err)
			}
		}
	}
	if err := flush(); err != nil {
		return itemCount, fmt.Errorf("final batch write: %w", err)
	}

	return itemCount, nil
}
