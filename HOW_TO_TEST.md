# How to test what's built so far

This covers what actually works today (`9c0dd83`). It'll grow as more of
`docs/design.md` gets implemented — check the date/commit below against
`git log` if it's been a while.

**Last updated:** 2026-08-13, commit `9c0dd83`

## Prerequisites

- Go 1.23+ (`go version`)
- Docker Desktop running (`docker info` should succeed, not hang)
- GNU Make (`make --version`)

## 1. Build, vet, unit tests

```
make build   # go build ./...
make lint    # go vet ./... + gofmt -l . (golangci-lint not set up yet)
make test    # go test ./...
```

`make test` currently only exercises `internal/extract` (heading
classification + full segmentation, both the HTML and Lever-`lists`
paths). Everything else has no unit tests yet — `internal/connectors`,
`internal/store`, etc. are only exercised by the live run below.

## 2. Bring up DynamoDB Local

```
make up      # docker compose up -d
```

Runs DynamoDB Local in `-inMemory` mode — **data does not persist** across
`docker compose down` or a container restart. That's intentional for local
dev (see PROGRESS.md's hang postmortem for why); `make ingest` repopulates
it every time. If `make up` hangs, Docker Desktop itself probably isn't
ready yet — `docker info` should return instantly when it is.

## 3. Run the ingest CLI against real companies

```
make ingest   # go run ./cmd/ingest ingest (runs `make up` first)
```

This hits the real Greenhouse and Lever public APIs for the 5 companies in
`data/companies.yaml` (Epic Games, Riot Games, Discord, Roblox, Kabam) —
no mocking, no fixtures. Expect structured log output like:

```
time=... level=INFO msg="company starting" index=1 of=5 company=epic-games ats=greenhouse
time=... level=INFO msg="greenhouse: fetching" company=epic-games url="https://boards-api.greenhouse.io/..."
time=... level=INFO msg="greenhouse: fetch complete" company=epic-games jobs=159 elapsed=267ms
time=... level=INFO msg="company complete" company=epic-games fetched=159 roleMatched=9 new=9 updated=0
...
time=... level=INFO msg="ingest run complete" companies=5 skipped=0 fetched=616 roleMatched=71 new=71 updated=0
```

A rerun should show `new=0` and `updated=` some number, since postings get
upserted, not duplicated. A company timing out or failing shows up as an
`ERROR` line with elapsed time and the underlying error — it gets skipped,
not fatal to the whole run.

**Env var overrides** (all optional):
| Var | Default | Purpose |
|---|---|---|
| `DYNAMODB_ENDPOINT` | `http://localhost:8000` | Where to find DynamoDB Local |
| `COMPANIES_FILE` | `data/companies.yaml` | Company registry to ingest |
| `INGEST_TIMEOUT` | `900` (15m, in seconds) | Overall run deadline |
| `COMPANY_TIMEOUT` | `20` (seconds) | Per-company fetch deadline before skip |

## 4. Run the report

```
make report   # go run ./cmd/ingest report
```

**This is interim, not the real deliverable.** It prints posting counts
per company:

```
=== Posting counts by company (interim — skill ranking pending extraction pipeline) ===
riot-games               26
roblox                   24
epic-games                9
discord                   7
kabam                     5

total postings: 71 across 5 companies
```

The actual DoD (ranked skill-frequency table) needs Stage 3 (dictionary
matching against `data/skills.yaml`), which isn't built yet — see
NEXT_STEPS.md. Once it lands, this section gets rewritten to show what
`make report` actually prints.

## 5. Inspecting DynamoDB Local directly (optional, for debugging)

If you have the AWS CLI installed, you can poke at the table directly —
credentials can be anything since DynamoDB Local doesn't check them:

```
aws dynamodb scan --table-name jobsyllabus --endpoint-url http://localhost:8000 --region us-west-2
```

## Tear down

```
make down    # docker compose down
```

(`make clean` also removes volumes, but there aren't any right now since
DynamoDB Local runs in-memory.)
