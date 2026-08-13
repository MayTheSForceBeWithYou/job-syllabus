.PHONY: build test lint fmt up down ingest report clean

build:
	go build ./...

test:
	go test ./...

# go vet + gofmt -l stand in for golangci-lint for now (not installed this
# session); swap in golangci-lint once it's added, per docs/phase-0.md.
lint:
	go vet ./...
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	fi

fmt:
	gofmt -w .

up:
	docker compose up -d
	@echo "waiting for DynamoDB Local..."
	@sleep 2

down:
	docker compose down

ingest: up
	go run ./cmd/ingest ingest

report:
	go run ./cmd/ingest report

clean:
	docker compose down -v
