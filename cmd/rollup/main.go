// cmd/rollup carries the whole-table backup/restore pair from
// docs/design.md §9 (the dev-data teardown safety net): `export` snapshots
// every item to S3, `import` restores from the latest snapshot. `reconcile`
// is Phase 4's write-time-counter half of §4 "the aggregation problem":
// counters drift — from failed transactions, deleted/closed postings,
// schema changes — so this recounts STAT#<roleFamily>/SKILL#<sid> from the
// actual PostingSkill edges (the source of truth) and corrects any
// counter that doesn't match, rather than trusting the write-time
// increments to have stayed perfectly accurate forever. `reextract` is
// Phase 5's re-extraction job (docs/design.md §6 "Re-extraction"): bumping
// extract.Version (e.g. after a review-queue approval) means the existing
// corpus was extracted under an older dictionary/prompt, so this
// re-enqueues every posting whose stamped ExtractVer is behind current —
// cmd/worker's bullet-level Bedrock cache means most of a re-run costs
// nothing, since the underlying bullet text hasn't changed even though the
// dictionary has.
//
// Usage:
//
//	go run ./cmd/rollup export
//	go run ./cmd/rollup import [s3-key]   # defaults to the latest export
//	go run ./cmd/rollup reconcile
//	go run ./cmd/rollup reextract
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
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/backup"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/extract"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/queue"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: rollup [export|import|reconcile|reextract] [s3-key]")
	}

	// reextract can run against a large corpus (every posting behind the
	// current ExtractVer, not just one day's ingest) and only sends SQS
	// messages rather than waiting on anything slow itself, so 5 minutes —
	// sized for export/import's whole-table Scan — comfortably covers it
	// too; revisit if the corpus grows enough for that to stop being true.
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

	switch os.Args[1] {
	case "export":
		bucket := requireBackupBucket()
		runExport(ctx, ddb, s3.NewFromConfig(cfg), bucket)
	case "import":
		bucket := requireBackupBucket()
		key := ""
		if len(os.Args) > 2 {
			key = os.Args[2]
		}
		runImport(ctx, ddb, s3.NewFromConfig(cfg), bucket, key)
	case "reconcile":
		runReconcile(ctx, store.NewFromClient(ddb))
	case "reextract":
		var sqsOpts []func(*sqs.Options)
		if endpoint := os.Getenv("SQS_ENDPOINT"); endpoint != "" {
			sqsOpts = append(sqsOpts, func(o *sqs.Options) { o.BaseEndpoint = aws.String(endpoint) })
		}
		runReextract(ctx, store.NewFromClient(ddb), sqs.NewFromConfig(cfg, sqsOpts...), requireExtractQueueURL())
	default:
		log.Fatalf("unknown subcommand %q (want export|import|reconcile|reextract)", os.Args[1])
	}
}

func requireExtractQueueURL() string {
	url := os.Getenv("EXTRACT_QUEUE_URL")
	if url == "" {
		log.Fatal("EXTRACT_QUEUE_URL must be set (modules/queues' extract-queue URL)")
	}
	return url
}

func requireBackupBucket() string {
	bucket := os.Getenv("BACKUP_BUCKET")
	if bucket == "" {
		log.Fatal("BACKUP_BUCKET must be set (the dev-data stack's data_bucket output)")
	}
	return bucket
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

// runReconcile recomputes every STAT#<roleFamily>/SKILL#<sid> counter from
// the actual PostingSkill edges belonging to active (non-closed) postings
// — the same "active" definition GET /v1/skills and `ingest report` use —
// and corrects any counter that's drifted from its write-time value.
// Closed postings' edges are deliberately excluded from the recount even
// though the edge items themselves aren't deleted (docs/design.md §4's
// lifecycle rule): a counter is a live-corpus statistic, not a historical
// archive.
func runReconcile(ctx context.Context, s *store.Store) {
	postings, err := s.ListAllPostings(ctx)
	if err != nil {
		log.Fatalf("list postings: %v", err)
	}
	roleFamilyByPostingID := make(map[string]model.RoleFamily, len(postings))
	for _, p := range postings {
		if p.ClosedAt == nil {
			roleFamilyByPostingID[p.ID] = p.RoleFamily
		}
	}

	edges, err := s.ListAllSkillEdges(ctx)
	if err != nil {
		log.Fatalf("list skill edges: %v", err)
	}

	type statKey struct {
		roleFamily model.RoleFamily
		skillID    string
	}
	actual := make(map[statKey]int)
	for _, e := range edges {
		roleFamily, active := roleFamilyByPostingID[e.PostingID]
		if !active {
			continue
		}
		actual[statKey{roleFamily, e.SkillID}]++
	}

	stored, err := s.ListAllSkillStats(ctx)
	if err != nil {
		log.Fatalf("list skill stats: %v", err)
	}
	storedCount := make(map[statKey]int, len(stored))
	for _, st := range stored {
		storedCount[statKey{st.RoleFamily, st.SkillID}] = st.Count
	}

	corrected := 0
	for k, want := range actual {
		if storedCount[k] == want {
			continue
		}
		if err := s.SetSkillStat(ctx, k.roleFamily, k.skillID, want); err != nil {
			log.Fatalf("correct stat %s/%s: %v", k.roleFamily, k.skillID, err)
		}
		fmt.Printf("corrected %s/%s: %d -> %d\n", k.roleFamily, k.skillID, storedCount[k], want)
		corrected++
	}
	// A counter present in the store but with zero current edges (every
	// posting that contributed to it was since closed, or the skill was
	// deprecated) needs to be zeroed too, not just left stale.
	for k := range storedCount {
		if _, stillActive := actual[k]; stillActive {
			continue
		}
		if storedCount[k] == 0 {
			continue
		}
		if err := s.SetSkillStat(ctx, k.roleFamily, k.skillID, 0); err != nil {
			log.Fatalf("zero stat %s/%s: %v", k.roleFamily, k.skillID, err)
		}
		fmt.Printf("corrected %s/%s: %d -> 0\n", k.roleFamily, k.skillID, storedCount[k])
		corrected++
	}

	fmt.Printf("reconciled %d skill counters (%d active postings, %d edges), %d corrected\n",
		len(actual), len(roleFamilyByPostingID), len(edges), corrected)
}

// runReextract re-enqueues every posting whose stamped ExtractVer is behind
// extract.Version — docs/design.md §6 "Re-extraction". Closed postings are
// skipped: they're already excluded from every active stat (GET /v1/skills,
// /v1/stats/overview, `report`), so spending worker cycles improving their
// extraction accuracy has no visible effect, mirroring runReconcile's own
// "active postings only" scope. Re-enqueuing just resends the existing
// RawS3Key cmd/ingest already uploaded — no need to refetch anything from
// the source ATS.
func runReextract(ctx context.Context, s *store.Store, sqsc *sqs.Client, queueURL string) {
	postings, err := s.ListAllPostings(ctx)
	if err != nil {
		log.Fatalf("list postings: %v", err)
	}

	enqueued, skippedClosed, skippedNoRaw := 0, 0, 0
	for _, p := range postings {
		if p.ExtractVer >= extract.Version {
			continue
		}
		if p.ClosedAt != nil {
			skippedClosed++
			continue
		}
		if p.RawS3Key == "" {
			// Shouldn't happen post-Phase-4 (every posting gets a raw S3
			// key at ingest time) but a posting from before that existed
			// would have none — nothing to re-extract from.
			skippedNoRaw++
			continue
		}
		if err := queue.SendExtractMessage(ctx, sqsc, queueURL, queue.ExtractMessage{PostingID: p.ID, RawS3Key: p.RawS3Key}); err != nil {
			log.Fatalf("enqueue reextract for %s: %v", p.ID, err)
		}
		enqueued++
	}

	fmt.Printf("reextract: %d postings scanned, %d enqueued (ExtractVer < %d), %d skipped (closed), %d skipped (no raw content)\n",
		len(postings), enqueued, extract.Version, skippedClosed, skippedNoRaw)
}
