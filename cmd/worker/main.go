// cmd/worker consumes extract-queue (docs/design.md §6/§9): for each
// message it loads the posting's metadata from DynamoDB and its raw body
// from S3 (internal/rawstore — DynamoDB has no body field), runs Stages
// 1-3 extraction (normalize/segment/dictionary match), Stage 4 (Bedrock
// fallback for zero-dictionary-hit bullets, Phase 5), and Stage 5 (unknown
// terms to the review queue, Phase 5), writes the resulting skill edges
// with idempotent write-time counters, and deletes the message. SQS is
// at-least-once delivery, so the same posting can be processed twice
// (redelivery after a crash, a slow run outliving its visibility timeout)
// — that's exactly why internal/store.PutSkillEdge is transactionally
// idempotent rather than a plain upsert.
//
// Stage 4 batches unmatched bullets across the whole batch of messages one
// ReceiveMessage call returns (up to 10), not per-message — docs/design.md
// §6 asks for "20 unmatched bullets per call", and at this corpus's real
// rate of ~3 unmatched bullets/posting, batching within one message would
// barely batch at all.
//
// Real AWS by default; DYNAMODB_ENDPOINT/S3_ENDPOINT/SQS_ENDPOINT override
// for local testing against DynamoDB Local + LocalStack, same convention
// cmd/ingest uses. BEDROCK_ENABLED=false (set by `make worker`'s local
// LOCAL_AWS_ENV) skips Stage 4 entirely, since LocalStack's fake
// credentials can't authenticate against real Bedrock and Bedrock has no
// local emulation.
package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/bedrock"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/config"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/extract"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/model"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/queue"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/rawstore"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/store"
)

// receiveBatchSize is SQS's own per-call maximum.
const receiveBatchSize = 10

// skillReloadInterval controls how often the worker re-merges
// data/skills.yaml with DynamoDB-approved skills (Phase 5's review-queue
// writeback) — short enough that an operator approving a term from their
// phone sees it take effect within one coffee break, long enough not to
// scan the table on every message batch.
const skillReloadInterval = 5 * time.Minute

// job is one posting moving through Stages 1-5 within a single receive
// batch. A nil dictMatches/unmatched after Stage 1-3 with failed=true means
// an earlier stage errored — the message is left in-flight for redelivery
// (matches the existing per-message error handling from Phase 4) and the
// rest of this job is skipped for Stage 4/5 and finalization.
type job struct {
	msg         queue.ReceivedMessage
	posting     *model.Posting
	dictMatches []model.PostingSkill
	unmatched   []extract.UnmatchedBullet
	llmFindings [][]bedrock.Finding // parallel to unmatched, filled by classifyUnmatched
	failed      bool
	skip        bool // posting no longer exists — message already deleted, nothing left to do
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ECS sends SIGTERM on deploy/scale-in/scale-to-zero — finish the
	// in-flight ReceiveMessage call and exit the loop instead of being
	// SIGKILLed mid-poll (same pattern cmd/api's HTTP server shutdown uses).
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		logger.Info("worker: shutdown signal received")
		cancel()
	}()

	startupCtx, startupCancel := context.WithTimeout(ctx, 30*time.Second)
	cfg, err := awsconfig.LoadDefaultConfig(startupCtx)
	startupCancel()
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}

	var ddbOpts []func(*dynamodb.Options)
	if endpoint := os.Getenv("DYNAMODB_ENDPOINT"); endpoint != "" {
		ddbOpts = append(ddbOpts, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(endpoint) })
	}
	s := store.NewFromClient(dynamodb.NewFromConfig(cfg, ddbOpts...))

	var s3Opts []func(*s3.Options)
	if endpoint := os.Getenv("S3_ENDPOINT"); endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) { o.BaseEndpoint = aws.String(endpoint); o.UsePathStyle = true })
	}
	s3c := s3.NewFromConfig(cfg, s3Opts...)

	var sqsOpts []func(*sqs.Options)
	if endpoint := os.Getenv("SQS_ENDPOINT"); endpoint != "" {
		sqsOpts = append(sqsOpts, func(o *sqs.Options) { o.BaseEndpoint = aws.String(endpoint) })
	}
	sqsc := sqs.NewFromConfig(cfg, sqsOpts...)

	rawBucket := os.Getenv("RAW_BUCKET")
	if rawBucket == "" {
		log.Fatal("RAW_BUCKET must be set (the raw-content S3 bucket, docs/design.md §9)")
	}
	extractQueueURL := os.Getenv("EXTRACT_QUEUE_URL")
	if extractQueueURL == "" {
		log.Fatal("EXTRACT_QUEUE_URL must be set (modules/queues' extract-queue URL)")
	}
	bedrockEnabled := envOr("BEDROCK_ENABLED", "true") != "false"
	bc := newBedrockClientOrNil(cfg, bedrockEnabled)

	skillsPath := envOr("SKILLS_FILE", "data/skills.yaml")
	yamlSkills, err := config.LoadSkills(skillsPath)
	if err != nil {
		log.Fatalf("load %s: %v", skillsPath, err)
	}

	initialLoadCtx, initialLoadCancel := context.WithTimeout(ctx, 30*time.Second)
	skills, lastReload := reloadSkills(initialLoadCtx, s, yamlSkills, logger)
	initialLoadCancel()

	logger.Info("worker: starting", "skills", len(skills), "queue", extractQueueURL, "bedrockEnabled", bedrockEnabled)

	for {
		if ctx.Err() != nil {
			logger.Info("worker: stopped")
			return
		}

		if time.Since(lastReload) > skillReloadInterval {
			skills, lastReload = reloadSkills(ctx, s, yamlSkills, logger)
		}

		msgs, err := queue.ReceiveExtractMessages(ctx, sqsc, extractQueueURL, receiveBatchSize)
		if err != nil {
			if ctx.Err() != nil {
				return // shutting down mid-receive, not a real error
			}
			logger.Error("worker: receive failed", "error", err.Error())
			continue
		}
		if len(msgs) == 0 {
			continue
		}

		processBatch(ctx, s, s3c, sqsc, bc, rawBucket, extractQueueURL, skills, msgs, logger)
	}
}

// reloadSkills re-merges the yaml seed with DynamoDB-approved skills
// (config.MergeSkills) and recompiles the matcher set. A DynamoDB read
// failure logs and falls back to whatever skills were already loaded
// (yaml-only on the very first call) rather than blocking extraction on a
// transient store error.
func reloadSkills(ctx context.Context, s *store.Store, yamlSkills []model.Skill, logger *slog.Logger) ([]extract.CompiledSkill, time.Time) {
	dynamic, err := s.ListDynamicSkills(ctx)
	if err != nil {
		logger.Error("worker: failed to list dynamic skills, keeping previous dictionary", "error", err.Error())
		compiled, compileErr := extract.CompileSkills(yamlSkills)
		if compileErr != nil {
			log.Fatalf("compile %d yaml skills: %v", len(yamlSkills), compileErr)
		}
		return compiled, time.Now()
	}

	merged := config.MergeSkills(yamlSkills, dynamic)
	compiled, err := extract.CompileSkills(merged)
	if err != nil {
		logger.Error("worker: failed to compile merged dictionary, keeping previous dictionary", "error", err.Error())
		compiled, err = extract.CompileSkills(yamlSkills)
		if err != nil {
			log.Fatalf("compile %d yaml skills: %v", len(yamlSkills), err)
		}
		return compiled, time.Now()
	}

	if len(dynamic) > 0 {
		logger.Info("worker: reloaded dictionary", "yamlSkills", len(yamlSkills), "dynamicSkills", len(dynamic))
	}
	return compiled, time.Now()
}

// processBatch runs Stages 1-3 for every message, then Stage 4 across the
// whole batch's unmatched bullets together, then Stage 5 + finalization per
// job. Any job that fails Stage 1-3 is left for redelivery exactly as
// before Phase 5 — Stage 4/5 simply skip it.
func processBatch(ctx context.Context, s *store.Store, s3c *s3.Client, sqsc *sqs.Client, bc *bedrock.Client, rawBucket, queueURL string, skills []extract.CompiledSkill, msgs []queue.ReceivedMessage, logger *slog.Logger) {
	jobs := make([]*job, len(msgs))
	for i, msg := range msgs {
		jobs[i] = runStagesOneToThree(ctx, s, s3c, sqsc, rawBucket, queueURL, skills, msg, logger)
	}

	classifyUnmatched(ctx, s, bc, jobs, logger)

	for _, j := range jobs {
		if j.skip || j.failed {
			continue
		}
		finalizeJob(ctx, s, sqsc, queueURL, skills, j, logger)
	}
}

// runStagesOneToThree loads a posting + its raw content and runs
// normalize/segment/dictionary-match, returning a job ready for Stage 4.
func runStagesOneToThree(ctx context.Context, s *store.Store, s3c *s3.Client, sqsc *sqs.Client, rawBucket, queueURL string, skills []extract.CompiledSkill, msg queue.ReceivedMessage, logger *slog.Logger) *job {
	j := &job{msg: msg}

	posting, err := s.GetPosting(ctx, msg.PostingID)
	if err != nil {
		logger.Error("worker: failed to load posting", "postingId", msg.PostingID, "error", err.Error())
		j.failed = true
		return j
	}
	if posting == nil {
		// Deleted (or never existed under this ID) — nothing to extract,
		// and no from-scratch retry will change that. Ack and move on.
		logger.Warn("worker: posting no longer exists, dropping message", "postingId", msg.PostingID)
		if err := queue.DeleteMessage(ctx, sqsc, queueURL, msg.ReceiptHandle); err != nil {
			logger.Error("worker: failed to delete message", "postingId", msg.PostingID, "error", err.Error())
		}
		j.skip = true
		return j
	}
	j.posting = posting

	rp, err := rawstore.Get(ctx, s3c, rawBucket, msg.RawS3Key)
	if err != nil {
		logger.Error("worker: failed to load raw content", "postingId", msg.PostingID, "error", err.Error())
		j.failed = true
		return j
	}

	sections, err := extract.SegmentPosting(rp)
	if err != nil {
		logger.Error("worker: segmentation failed", "postingId", msg.PostingID, "error", err.Error())
		j.failed = true
		return j
	}

	j.dictMatches = extract.MatchSkills(posting.ID, sections, skills)
	j.unmatched = extract.FindUnmatchedBullets(sections, skills)
	return j
}

// finalizeJob is Stage 5 (resolve each Bedrock finding against the current
// dictionary, or route it to the review queue) plus writing every edge and
// stamping/deleting the message.
func finalizeJob(ctx context.Context, s *store.Store, sqsc *sqs.Client, queueURL string, skills []extract.CompiledSkill, j *job, logger *slog.Logger) {
	matches := j.dictMatches
	seen := make(map[string]bool, len(matches))
	for _, m := range matches {
		seen[m.SkillID] = true
	}

	for bi, findings := range j.llmFindings {
		if bi >= len(j.unmatched) {
			continue
		}
		required := j.unmatched[bi].Required
		for _, f := range findings {
			if skillID, ok := resolveFinding(f, skills); ok {
				if seen[skillID] {
					continue
				}
				seen[skillID] = true
				evidence := f.Evidence
				if len(evidence) > 200 {
					evidence = evidence[:200]
				}
				matches = append(matches, model.PostingSkill{
					PostingID:  j.posting.ID,
					SkillID:    skillID,
					Required:   required,
					Evidence:   evidence,
					Confidence: f.Confidence,
					Method:     "llm",
				})
				continue
			}

			if _, err := s.RecordReviewOccurrence(ctx, f.Term, f.Category, f.Evidence); err != nil {
				logger.Error("worker: failed to record review occurrence", "term", f.Term, "postingId", j.posting.ID, "error", err.Error())
			}
		}
	}

	for _, m := range matches {
		if err := s.PutSkillEdge(ctx, j.posting.RoleFamily, m); err != nil {
			logger.Error("worker: failed to store skill edge", "postingId", j.posting.ID, "skillId", m.SkillID, "error", err.Error())
			return
		}
	}

	j.posting.ExtractVer = extract.Version
	j.posting.SkillCount = len(matches)
	if err := s.PutPosting(ctx, *j.posting); err != nil {
		logger.Error("worker: failed to stamp extraction fields", "postingId", j.posting.ID, "error", err.Error())
		return
	}

	if err := queue.DeleteMessage(ctx, sqsc, queueURL, j.msg.ReceiptHandle); err != nil {
		logger.Error("worker: failed to delete message", "postingId", j.posting.ID, "error", err.Error())
		return
	}

	dictCount := len(j.dictMatches)
	logger.Info("worker: extracted", "postingId", j.posting.ID, "skills", len(matches), "dict", dictCount, "llm", len(matches)-dictCount, "unmatchedBullets", len(j.unmatched))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
