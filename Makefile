.PHONY: build test lint fmt up down ingest report clean

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
	@sleep 2

down:
	docker compose down

ingest: up
	go run ./cmd/ingest ingest

report:
	go run ./cmd/ingest report

clean:
	docker compose down -v
