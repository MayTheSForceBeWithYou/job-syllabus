.PHONY: build test lint fmt up down ingest worker report clean

# GNU Make on Windows has no sleep(1). Prefer PowerShell; fall back to Unix sleep.
ifeq ($(OS),Windows_NT)
WAIT = powershell -NoProfile -Command "Start-Sleep -Seconds 5"
else
WAIT = sleep 5
endif

# Phase 4's real-AWS-by-default cmd/ingest/cmd/worker need these overrides
# to run locally at all (DYNAMODB_ENDPOINT alone isn't enough anymore) —
# LocalStack (docker-compose.yml) stands in for SQS + S3. Any credentials
# work; LocalStack's community edition doesn't check them. BEDROCK_ENABLED=
# false (Phase 5) skips Stage 4 entirely — these fake credentials can't
# authenticate against real Bedrock (LocalStack doesn't emulate it either),
# so `make worker` would otherwise just log a Bedrock auth error on every
# batch with unmatched bullets.
LOCAL_AWS_ENV = AWS_ACCESS_KEY_ID=local AWS_SECRET_ACCESS_KEY=local AWS_REGION=us-west-1 \
	DYNAMODB_ENDPOINT=http://localhost:8000 S3_ENDPOINT=http://localhost:4566 SQS_ENDPOINT=http://localhost:4566 \
	RAW_BUCKET=job-syllabus-raw-local \
	EXTRACT_QUEUE_URL=http://sqs.us-west-1.localhost.localstack.cloud:4566/000000000000/extract-queue \
	BEDROCK_ENABLED=false

build:
	go build ./...

test:
	go test ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

up:
	docker compose up -d
	@echo "waiting for DynamoDB Local + LocalStack..."
	@$(WAIT)
	@AWS_ACCESS_KEY_ID=local AWS_SECRET_ACCESS_KEY=local aws --endpoint-url=http://localhost:4566 --region us-west-1 \
		sqs create-queue --queue-name extract-queue >/dev/null 2>&1 || true
	@AWS_ACCESS_KEY_ID=local AWS_SECRET_ACCESS_KEY=local aws --endpoint-url=http://localhost:4566 --region us-west-1 \
		s3 mb s3://job-syllabus-raw-local >/dev/null 2>&1 || true

down:
	docker compose down

# Enqueues to LocalStack's extract-queue; run `make worker` in a second
# terminal to actually drain it and see extraction happen — `make ingest`
# alone only fetches/dedupes/upserts postings now (docs/phase-4.md).
ingest: up
	$(LOCAL_AWS_ENV) go run ./cmd/ingest ingest

# Long-polls the local extract-queue and runs Stages 1-3 until you Ctrl-C
# it — there's no "process what's queued and exit" mode, matching how it
# runs for real (a long-lived ECS service, not a one-shot job).
worker: up
	$(LOCAL_AWS_ENV) go run ./cmd/worker

report:
	go run ./cmd/ingest report

clean:
	docker compose down -v
