.PHONY: build test lint fmt up down ingest report clean

# GNU Make on Windows has no sleep(1). Prefer PowerShell; fall back to Unix sleep.
ifeq ($(OS),Windows_NT)
WAIT = powershell -NoProfile -Command "Start-Sleep -Seconds 2"
else
WAIT = sleep 2
endif

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
	@echo "waiting for DynamoDB Local..."
	@$(WAIT)

down:
	docker compose down

ingest: up
	go run ./cmd/ingest ingest

report:
	go run ./cmd/ingest report

clean:
	docker compose down -v
