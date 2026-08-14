# How to test what's built so far

This covers what actually works today (`ea09ef6`). It'll grow as more of
`docs/design.md` gets implemented — check the date/commit below against
`git log` if it's been a while.

**Last updated:** 2026-08-13, commit `ea09ef6` — Phase 0/1 DoD met.

## Prerequisites

- Go 1.23+ (`go version`)
- Docker Desktop running (`docker info` should succeed, not hang)
- golangci-lint v2.x (`golangci-lint --version`) — only needed for `make lint`
- GNU Make (`make --version`)

## 1. Build, lint, unit tests

```
make build   # go build ./...
make lint    # golangci-lint run ./... (config: .golangci.yml, standard set + unconvert/unparam)
make test    # go test ./...
```

`make lint` needs `golangci-lint` on PATH (v2.x — the config schema
changed between v1 and v2, so an older install won't read `.golangci.yml`
correctly).

`make test` currently only exercises `internal/extract` (heading
classification, both segmentation paths, and dictionary matching —
case-sensitivity handling, required-wins-over-nice_to_have, evidence
truncation). Everything else has no unit tests yet — `internal/connectors`,
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
time=... level=INFO msg="greenhouse: fetch complete" company=epic-games jobs=161 elapsed=223ms
time=... level=INFO msg="company complete" company=epic-games fetched=161 roleMatched=9 new=9 updated=0 skillEdges=37
...
time=... level=INFO msg="ingest run complete" companies=5 skipped=0 fetched=617 roleMatched=70 new=70 updated=0 skillEdges=169
```

A rerun should show `new=0` and `updated=` some number, since postings get
upserted, not duplicated. A company timing out or failing shows up as an
`ERROR` line with elapsed time and the underlying error — it gets skipped,
not fatal to the whole run. **Watch `skillEdges` per company** — a company
stuck at 0 while others aren't is the signal that its postings use section
headings the dictionary matcher isn't recognizing (this is exactly how the
Epic Games heading bug was found; see PROGRESS.md). `kabam` legitimately
stays at 0 — that's an accepted, documented gap (NEXT_STEPS.md), not a bug.

**Env var overrides** (all optional):
| Var | Default | Purpose |
|---|---|---|
| `DYNAMODB_ENDPOINT` | `http://localhost:8000` | Where to find DynamoDB Local |
| `COMPANIES_FILE` | `data/companies.yaml` | Company registry to ingest |
| `SKILLS_FILE` | `data/skills.yaml` | Skill dictionary for extraction and report |
| `INGEST_TIMEOUT` | `900` (15m, in seconds) | Overall run deadline |
| `COMPANY_TIMEOUT` | `20` (seconds) | Per-company fetch deadline before skip |

## 4. Run the report

```
make report   # go run ./cmd/ingest report
```

**This is the real deliverable** — the ranked skill-frequency table,
computed fresh from every stored `PostingSkill` edge each run:

```
=== Skill frequency across 70 postings (5 companies) ===
SKILL                        CATEGORY        COUNT % OF POSTS  REQ'D NICE-TO-HAVE
Amazon Web Services (AWS)    cloud              12      17.1%      9            3
Kubernetes                   containers         11      15.7%      8            3
Google Cloud Platform (GCP)  cloud              10      14.3%      8            2
C++                          languages           9      12.9%      8            1
...

41 distinct skills matched across 70 postings
```

Numbers will drift run to run as real postings open/close — this is a live
snapshot of the job market, not a fixture. If `report` shows 0 skills
entirely, check `ingest`'s log for `skillEdges` — that means extraction
found nothing anywhere, which would be a real regression (every company
except the accepted `kabam` gap should contribute something).

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
