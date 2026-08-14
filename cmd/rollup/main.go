// cmd/rollup carries the whole-table backup/restore pair from
// docs/design.md §9 (the dev-data teardown safety net): `export` snapshots
// every item to S3, `import` restores from the latest snapshot. Nightly
// aggregate-counter reconciliation (§4 "The aggregation problem") isn't
// built yet — Phase 1 has no write-time counters to reconcile against (see
// NEXT_STEPS.md).
//
// Usage:
//
//	go run ./cmd/rollup export
//	go run ./cmd/rollup import [s3-key]   # defaults to the latest export
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/backup"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: rollup [export|import] [s3-key]")
	}

	bucket := os.Getenv("BACKUP_BUCKET")
	if bucket == "" {
		log.Fatal("BACKUP_BUCKET must be set (the dev-data stack's data_bucket output)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}

	var ddbOpts []func(*dynamodb.Options)
	if endpoint := os.Getenv("DYNAMODB_ENDPOINT"); endpoint != "" {
		// Same override cmd/ingest uses — lets this run against DynamoDB
		// Local for testing without touching real AWS.
		ddbOpts = append(ddbOpts, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}
	ddb := dynamodb.NewFromConfig(cfg, ddbOpts...)
	s3c := s3.NewFromConfig(cfg)

	switch os.Args[1] {
	case "export":
		runExport(ctx, ddb, s3c, bucket)
	case "import":
		key := ""
		if len(os.Args) > 2 {
			key = os.Args[2]
		}
		runImport(ctx, ddb, s3c, bucket, key)
	default:
		log.Fatalf("unknown subcommand %q (want export|import)", os.Args[1])
	}
}

func runExport(ctx context.Context, ddb *dynamodb.Client, s3c *s3.Client, bucket string) {
	count, key, err := backup.Export(ctx, ddb, s3c, store.TableName, bucket)
	if err != nil {
		log.Fatalf("export: %v", err)
	}
	fmt.Printf("exported %d items to s3://%s/%s (and %s)\n", count, bucket, key, backup.LatestKey)
}

func runImport(ctx context.Context, ddb *dynamodb.Client, s3c *s3.Client, bucket, key string) {
	count, err := backup.Import(ctx, ddb, s3c, store.TableName, bucket, key)
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	used := key
	if used == "" {
		used = backup.LatestKey
	}
	fmt.Printf("imported %d items from s3://%s/%s into table %q\n", count, bucket, used, store.TableName)
}
