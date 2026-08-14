# How to test what's built so far

This covers what actually works today. It'll grow as more of
`docs/design.md` gets implemented — check the date below against
`git log` if it's been a while.

**Last updated:** 2026-08-14 — Phase 0/1/2 complete, DoD and §6 validation
gate both passing, real AWS infrastructure + Jenkins live. Sections 1-5
below (local pipeline) are unchanged from Phase 1; section 6 is new.

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

`make test` currently only exercises `internal/extract`: heading
classification, both segmentation paths (including the bold-paragraph
pseudo-heading pattern), dictionary matching (case-sensitivity handling,
required-wins-over-nice_to_have, evidence truncation), and the §6
hand-labeled precision/recall gate (`TestExtractionPrecisionRecall` — 70
real postings in `testdata/labeled/`, must stay ≥90% precision or the
build fails; see PROGRESS.md for the current number). Everything else has
no unit tests yet — `internal/connectors`, `internal/store`, etc. are only
exercised by the live run below.

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
headings (or an HTML structure) the extraction pipeline isn't recognizing.
This is exactly how the Epic Games and Roblox heading bugs were found; see
PROGRESS.md. `kabam` legitimately stays at 0 — that's an accepted,
documented gap (NEXT_STEPS.md), not a bug.

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
SKILL                        CATEGORY     COUNT % OF POSTS  REQ'D NICE-TO-HAVE
Amazon Web Services (AWS)    cloud           16      22.9%     12            4
Kubernetes                   containers      16      22.9%     12            4
Google Cloud Platform (GCP)  cloud           14      20.0%     11            3
C++                          languages       13      18.6%     12            1
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

## 6. Testing the real infrastructure (Phase 2)

This needs AWS CLI credentials configured for the same account bootstrap
ran against, and Terraform on `PATH`.

**Jenkins**: `https://jenkins.job-syllabus.skopekreep.com` — admin
username `admin`, password from SSM:

```
aws ssm get-parameter --name /job-syllabus/jenkins/admin-password --with-decryption --region us-west-1 --query Parameter.Value --output text
```

Three jobs should be visible: `api-build`, `infra-plan`, `infra-apply`
(seeded by the Job DSL script in `ci/jobs.groovy` — not created by hand).

**Terraform stacks** (`infra/terraform/envs/{dev-data,dev-compute}`):
standard `terraform init && terraform plan` in either directory. `dev-data`
should almost never show drift (it's long-lived); `dev-compute` is
designed to be destroyed and reapplied at will for cost control —
`terraform destroy` there, followed by `terraform apply`, is the actual
Phase 2 DoD test (see `docs/phase-2.md`) and should always end with a
healthy Jenkins and no manual steps.

**Diagnosing a boot that doesn't come up clean**: don't trust
`terraform apply`'s exit code alone — poll the instance directly via SSM
(no SSH needed, works through the IAM instance role):

```
aws ssm send-command --instance-ids <id> --document-name "AWS-RunShellScript" \
  --parameters 'commands=["systemctl is-active jenkins","tail -40 /var/log/cloud-init-output.log"]' \
  --region us-west-1 --query 'Command.CommandId' --output text
# then, after a few seconds:
aws ssm get-command-invocation --command-id <command-id> --instance-id <id> --region us-west-1 --query StandardOutputContent --output text
```

If the AWS CLI's own output garbles with `'charmap' codec can't encode`
errors on Windows, that's a local console-encoding issue, not a remote
failure — set `PYTHONUTF8=1` first.
