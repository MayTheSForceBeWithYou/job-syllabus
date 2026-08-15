// cmd/api serves the read-only REST API from docs/design.md §7 (Phase 3:
// "Deploy API"). Real DynamoDB against real AWS by default;
// DYNAMODB_ENDPOINT overrides for local testing against DynamoDB Local —
// same convention cmd/rollup already uses.
package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/api"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/config"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/connectors"
	"github.com/MayTheSForceBeWithYou/job-syllabus/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startupCancel()

	cfg, err := awsconfig.LoadDefaultConfig(startupCtx)
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}

	var ddbOpts []func(*dynamodb.Options)
	if endpoint := os.Getenv("DYNAMODB_ENDPOINT"); endpoint != "" {
		ddbOpts = append(ddbOpts, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}
	s := store.NewFromClient(dynamodb.NewFromConfig(cfg, ddbOpts...))

	companiesPath := envOr("COMPANIES_FILE", "data/companies.yaml")
	skillsPath := envOr("SKILLS_FILE", "data/skills.yaml")

	httpClient := connectors.NewDefaultHTTPClient()
	registry := connectors.NewRegistry(httpClient)
	companies, err := config.LoadCompanies(companiesPath, registry)
	if err != nil {
		log.Fatalf("load %s: %v", companiesPath, err)
	}
	skills, err := config.LoadSkills(skillsPath)
	if err != nil {
		log.Fatalf("load %s: %v", skillsPath, err)
	}
	logger.Info("api: loaded config", "companies", len(companies), "skills", len(skills))

	srv := api.New(s, skills, companies, os.Getenv("ALLOWED_CIDR"))
	if err := srv.RefreshSkills(startupCtx); err != nil {
		// Not fatal — proceed with the yaml-only dictionary and let the
		// background refresh loop below retry; a transient DynamoDB error
		// at startup shouldn't block the whole API from serving.
		logger.Error("api: initial skill refresh failed, starting with yaml-only dictionary", "error", err.Error())
	}
	go runSkillRefreshLoop(context.Background(), srv, logger)

	addr := ":" + envOr("PORT", "8080")
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv.Router(),
		// Explicit timeouts throughout — same "never rely on a client/
		// server without a stated timeout" rule the ATS connectors and
		// store package already follow (see PROGRESS.md's 34-minute-hang
		// postmortem).
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("api: listening", "addr", addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	// ECS sends SIGTERM on deploy/scale-in and waits stopTimeout (default
	// 30s) before SIGKILL — finish in-flight requests instead of
	// dropping them mid-response.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Info("api: shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("api: graceful shutdown failed", "error", err.Error())
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// skillRefreshInterval matches cmd/worker's own reload cadence — an
// operator approving a review-queue term should see /v1/skills reflect it
// within about the same window regardless of which service they check
// first.
const skillRefreshInterval = 5 * time.Minute

// runSkillRefreshLoop periodically re-merges data/skills.yaml with
// DynamoDB-approved skills (Phase 5's review-queue writeback, docs/
// design.md §6 Stage 5) so an approval takes effect without redeploying
// service-api. Runs until the process exits — there's no graceful-shutdown
// signal to wire this to since it does no I/O worth draining, unlike the
// HTTP server's own shutdown handling above.
func runSkillRefreshLoop(ctx context.Context, srv *api.Server, logger *slog.Logger) {
	ticker := time.NewTicker(skillRefreshInterval)
	defer ticker.Stop()
	for range ticker.C {
		refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := srv.RefreshSkills(refreshCtx)
		cancel()
		if err != nil {
			logger.Error("api: periodic skill refresh failed", "error", err.Error())
		}
	}
}
