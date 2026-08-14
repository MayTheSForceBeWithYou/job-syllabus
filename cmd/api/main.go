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

	srv := api.New(s, skills, companies)

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
