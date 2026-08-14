// cmd/worker consumes extract-queue (docs/design.md §6/§9): for each
// message it loads the posting's metadata from DynamoDB and its raw body
// from S3 (internal/rawstore — DynamoDB has no body field), runs Stages
// 1-3 extraction (normalize/segment/dictionary match), writes the
// resulting skill edges with idempotent write-time counters, and deletes
// the message. SQS is at-least-once delivery, so the same posting can be
// processed twice (redelivery after a crash, a slow run outliving its
// visibility timeout) — that's exactly why internal/store.PutSkillEdge is
// transactionally idempotent rather than a plain upsert.
//
// Real AWS by default; DYNAMODB_ENDPOINT/S3_ENDPOINT/SQS_ENDPOINT override
// for local testing against DynamoDB Local + LocalStack, same convention
// cmd/ingest uses.
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

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/config"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/extract"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/queue"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/rawstore"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/store"
)

// receiveBatchSize is SQS's own per-call maximum.
const receiveBatchSize = 10

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

	skillsPath := envOr("SKILLS_FILE", "data/skills.yaml")
	rawSkills, err := config.LoadSkills(skillsPath)
	if err != nil {
		log.Fatalf("load %s: %v", skillsPath, err)
	}
	skills, err := extract.CompileSkills(rawSkills)
	if err != nil {
		log.Fatalf("compile %s: %v", skillsPath, err)
	}

	logger.Info("worker: starting", "skills", len(skills), "queue", extractQueueURL)

	for {
		if ctx.Err() != nil {
			logger.Info("worker: stopped")
			return
		}

		msgs, err := queue.ReceiveExtractMessages(ctx, sqsc, extractQueueURL, receiveBatchSize)
		if err != nil {
			if ctx.Err() != nil {
				return // shutting down mid-receive, not a real error
			}
			logger.Error("worker: receive failed", "error", err.Error())
			continue
		}

		for _, msg := range msgs {
			processMessage(ctx, s, s3c, sqsc, rawBucket, extractQueueURL, skills, msg, logger)
		}
	}
}

// processMessage runs Stages 1-3 on one queued posting. Any failure
// before the final DeleteMessage leaves the message in-flight to be
// redelivered after the visibility timeout — safe because every write
// here (PutSkillEdge, PutPosting) is naturally or transactionally
// idempotent, so a from-scratch retry can't double-count or corrupt state.
func processMessage(ctx context.Context, s *store.Store, s3c *s3.Client, sqsc *sqs.Client, rawBucket, queueURL string, skills []extract.CompiledSkill, msg queue.ReceivedMessage, logger *slog.Logger) {
	posting, err := s.GetPosting(ctx, msg.PostingID)
	if err != nil {
		logger.Error("worker: failed to load posting", "postingId", msg.PostingID, "error", err.Error())
		return
	}
	if posting == nil {
		// Deleted (or never existed under this ID) — nothing to extract,
		// and no from-scratch retry will change that. Ack and move on.
		logger.Warn("worker: posting no longer exists, dropping message", "postingId", msg.PostingID)
		if err := queue.DeleteMessage(ctx, sqsc, queueURL, msg.ReceiptHandle); err != nil {
			logger.Error("worker: failed to delete message", "postingId", msg.PostingID, "error", err.Error())
		}
		return
	}

	rp, err := rawstore.Get(ctx, s3c, rawBucket, msg.RawS3Key)
	if err != nil {
		logger.Error("worker: failed to load raw content", "postingId", msg.PostingID, "error", err.Error())
		return
	}

	sections, err := extract.SegmentPosting(rp)
	if err != nil {
		logger.Error("worker: segmentation failed", "postingId", msg.PostingID, "error", err.Error())
		return
	}

	matches := extract.MatchSkills(posting.ID, sections, skills)
	for _, m := range matches {
		if err := s.PutSkillEdge(ctx, posting.RoleFamily, m); err != nil {
			logger.Error("worker: failed to store skill edge", "postingId", posting.ID, "skillId", m.SkillID, "error", err.Error())
			return
		}
	}

	posting.ExtractVer = extract.Version
	posting.SkillCount = len(matches)
	if err := s.PutPosting(ctx, *posting); err != nil {
		logger.Error("worker: failed to stamp extraction fields", "postingId", posting.ID, "error", err.Error())
		return
	}

	if err := queue.DeleteMessage(ctx, sqsc, queueURL, msg.ReceiptHandle); err != nil {
		logger.Error("worker: failed to delete message", "postingId", posting.ID, "error", err.Error())
		return
	}

	logger.Info("worker: extracted", "postingId", posting.ID, "skills", len(matches))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
