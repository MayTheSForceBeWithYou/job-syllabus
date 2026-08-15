# How to test what's built so far

This covers what actually works today. It'll grow as more of
`docs/design.md` gets implemented — check the date below against
`git log` if it's been a while.

**Last updated:** 2026-08-15 — Phase 0-4 complete, DoD and §6 validation
gate both passing, real AWS infrastructure + Jenkins + `service-api` +
queue-based ingestion all live. Sections 1-5 below (local pipeline) are
mostly unchanged from Phase 1, but note section 3's update — `cmd/ingest`
no longer extracts inline (Phase 4), so seeing real skill data locally
now needs `make worker` running too. Section 6 covers Phase 2's
infrastructure, section 7 Phase 3's deployed API, section 8 (new) Phase
4's real ingestion pipeline.

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

## 2. Bring up DynamoDB Local + LocalStack

```
make up      # docker compose up -d, then provisions the local SQS queue + S3 bucket
```

Runs DynamoDB Local in `-inMemory` mode — **data does not persist** across
`docker compose down` or a container restart. That's intentional for local
dev (see PROGRESS.md's hang postmortem for why); `make ingest` repopulates
it every time. As of Phase 4, `make up` also starts LocalStack (SQS + S3,
pinned to `3.0.2` — newer tags require a paid auth token even for these
free-tier services) and creates the `extract-queue` + `job-syllabus-raw-local`
bucket `cmd/ingest`/`cmd/worker` need. If `make up` hangs, Docker Desktop
itself probably isn't ready yet — `docker info` should return instantly
when it is.

## 3. Run the ingest CLI against real companies

```
make ingest   # fetches/dedupes/upserts postings, enqueues each for extraction
make worker   # in a second terminal: drains the queue, runs extraction, Ctrl-C when idle
```

This hits the real Greenhouse/Lever/Ashby/SmartRecruiters/Workable/Workday
APIs for all 49 companies in `data/companies.yaml` — no mocking, no
fixtures. **As of Phase 4, `cmd/ingest` no longer extracts inline** — it
only fetches, dedupes, upserts postings, and enqueues them; `cmd/worker`
is what actually runs Stages 1-3 and writes skill edges. Run both to see
real skill data locally. Expect structured log output like:

```
time=... level=INFO msg="company starting" index=1 of=49 company=epic-games ats=greenhouse
time=... level=INFO msg="greenhouse: fetching" company=epic-games url="https://boards-api.greenhouse.io/..."
time=... level=INFO msg="greenhouse: fetch complete" company=epic-games jobs=161 elapsed=223ms
time=... level=INFO msg="company complete" company=epic-games fetched=161 roleMatched=9 new=9 updated=0 closed=0 enqueued=9
...
time=... level=INFO msg="ingest run complete" companies=49 skipped=0 fetched=1735 roleMatched=162 new=0 updated=162 closed=0 enqueued=162
```

then, from `make worker`:

```
time=... level=INFO msg="worker: starting" skills=89 queue=http://sqs.us-west-1.localhost.localstack.cloud:4566/...
time=... level=INFO msg="worker: extracted" postingId=... skills=4
...
```

A rerun of `make ingest` should show `new=0` and `updated=` some number,
since postings get upserted, not duplicated — and re-running `make worker`
against the same postings should leave every skill count and edge count
unchanged (`internal/store.PutSkillEdge`'s idempotent `TransactWriteItems`,
docs/design.md §4). A company timing out or failing shows up as an
`ERROR` line with elapsed time and the underlying error — it gets skipped,
not fatal to the whole run. **Watch `skillEdges` per company in the
worker's logs** — a company stuck at 0 while others aren't is the signal
that its postings use section headings (or an HTML structure) the
extraction pipeline isn't recognizing. This is exactly how the Epic Games
and Roblox heading bugs were found; see PROGRESS.md. `kabam` legitimately
stays at 0 — that's an accepted, documented gap (NEXT_STEPS.md), not a
bug. Many of the other 44 companies also legitimately show 0 this run —
see docs/phase-4.md's honest accounting of real-world role-matched
volume, also not a bug.

**Env var overrides** (all optional unless noted):
| Var | Default | Purpose |
|---|---|---|
| `DYNAMODB_ENDPOINT` | (real AWS) | Where to find DynamoDB — `make ingest`/`make worker` set this to DynamoDB Local |
| `S3_ENDPOINT` | (real AWS) | Where to find S3 — set to LocalStack by `make ingest`/`make worker` |
| `SQS_ENDPOINT` | (real AWS) | Where to find SQS — set to LocalStack by `make ingest`/`make worker` |
| `RAW_BUCKET` | **required** for `ingest` | S3 bucket for raw posting content |
| `EXTRACT_QUEUE_URL` | **required** for `ingest`/`worker` | The extract-queue URL |
| `COMPANIES_FILE` | `data/companies.yaml` | Company registry to ingest |
| `SKILLS_FILE` | `data/skills.yaml` | Skill dictionary for extraction and report |
| `INGEST_TIMEOUT` | `900` (15m, in seconds) | Overall run deadline |
| `COMPANY_TIMEOUT` | `20` (seconds) | Per-company fetch deadline before skip |
| `INGEST_ONLY_COMPANY` | (unset = all) | Scope a run to one company's `slug` (backs the Jenkins `backfill` job) |

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

## 7. Testing the deployed API (Phase 3)

The real API Gateway URL is published to SSM (not hardcoded — it's an
apply-specific value):

```
API_URL=$(aws ssm get-parameter --name /job-syllabus/api/url --region us-west-1 --query Parameter.Value --output text)
curl "$API_URL/healthz"
curl "$API_URL/v1/skills?limit=10"
curl "$API_URL/v1/stats/overview"
```

`/healthz` and `/readyz` are unauthenticated; everything under `/v1/*` is
locked to the operator's IP (`internal/api/ipallow.go`, checking the
`X-Real-Ip` header API Gateway's integration injects from
`$context.identity.sourceIp` — not the conventional `X-Forwarded-For`,
which doesn't carry the real client IP through this API Gateway → VPC
Link → ALB path at all; see `docs/phase-3.md`). Requesting from a
different IP gets a `403 forbidden` RFC 7807 body.

**Redeploying**: push to `main` with changes under `cmd/**` or
`internal/**` — `api-build`'s `pollSCM` trigger picks it up within 5
minutes (a GitHub webhook can't reach this Jenkins; see the Jenkinsfile's
header comment), or trigger it manually from the Jenkins UI. The pipeline
builds with Kaniko on a dedicated `kaniko-agent` label, verifies the push
against ECR directly rather than trusting the build stage's own exit
status (see `docs/phase-3.md` for why), then deploys and smoke-tests
automatically.

**If the real DynamoDB table is ever empty** (a fresh `dev-data` apply, or
after a restore): `cmd/ingest` only supports DynamoDB Local, so populate
the real table via the same backup/restore pair Phase 2 built for the
`dev-data` teardown safety net —

```
make up && make ingest   # populate DynamoDB Local first
DYNAMODB_ENDPOINT=http://localhost:8000 BACKUP_BUCKET=job-syllabus-data-881811711506 AWS_REGION=us-west-1 go run ./cmd/rollup export
BACKUP_BUCKET=job-syllabus-data-881811711506 AWS_REGION=us-west-1 go run ./cmd/rollup import
```

**Diagnosing an API request that 403s unexpectedly**: `ipAllowlist` logs
the rejected request's actual header values via `slog` — check
CloudWatch, not just the response body:

```
aws logs filter-log-events --log-group-name /ecs/job-syllabus --region us-west-1 \
  --filter-pattern "\"ip allowlist rejected\"" --query 'events[].message' --output text
```

## 8. Testing the real ingestion pipeline (Phase 4)

This needs the same AWS CLI credentials as section 6/7. Unlike `service-api`,
none of `cmd/ingest`/`cmd/worker`/`cmd/rollup` sit behind API Gateway —
they're either a long-running Fargate Spot service (`worker`) or one-shot
tasks invoked by EventBridge Scheduler (`ingest`, `rollup reconcile`).

**Get the queue URL** (needed for several commands below):

```
QUEUE_URL=$(aws sqs get-queue-url --queue-name job-syllabus-extract-queue --region us-west-1 --query QueueUrl --output text)
DLQ_URL=$(aws sqs get-queue-url --queue-name job-syllabus-extract-dlq --region us-west-1 --query QueueUrl --output text)
```

**Check DLQ depth** — should always be 0 at rest. A non-zero count means
`worker` is failing repeatedly on some message (SQS `redrive_policy` sends
it there after 3 failed receives) and is exactly what the
`job-syllabus-extract-dlq-depth` CloudWatch alarm (`modules/observability`)
pages on:

```
aws sqs get-queue-attributes --queue-url $DLQ_URL --attribute-names ApproximateNumberOfMessages \
  --region us-west-1 --query 'Attributes.ApproximateNumberOfMessages' --output text
```

**Watch the worker autoscale with queue depth**: `modules/service-worker`
runs at `min_capacity=0` — no messages, no running tasks, no cost. Enqueue
work (see the manual ingest trigger below) and watch the service scale out,
then back to zero once the queue drains (StepScaling on
`ApproximateNumberOfMessagesVisible`, not the target-tracking CPU policy
`service-api` uses):

```
watch -n 10 'aws ecs describe-services --cluster job-syllabus --services worker --region us-west-1 \
  --query "services[0].[desiredCount,runningCount]" --output text; \
  aws sqs get-queue-attributes --queue-url '"$QUEUE_URL"' --attribute-names ApproximateNumberOfMessagesVisible \
  --region us-west-1 --query Attributes.ApproximateNumberOfMessagesVisible --output text'
```

(No `watch` on Windows — just re-run both commands every ~10s by hand, or
use `powershell -Command "while($true){...; Start-Sleep 10}"`.)

**Manually trigger the real ingest or reconcile task**, rather than waiting
for the daily 06:00/07:00 UTC EventBridge schedule — get the exact network
config the schedule itself uses so you don't have to guess subnets/security
groups:

```
aws scheduler get-schedule --name job-syllabus-ingest --region us-west-1
# copy the taskDefinition ARN + networkConfiguration out of the target, then:
aws ecs run-task --cluster job-syllabus --task-definition job-syllabus-ingest \
  --launch-type FARGATE --network-configuration '<paste networkConfiguration here>' --region us-west-1
```

Same pattern with schedule name `job-syllabus-rollup-reconcile` / task
definition family `job-syllabus-rollup-reconcile` for the reconcile task.
Follow the run with:

```
aws ecs wait tasks-stopped --cluster job-syllabus --tasks <task-arn-from-run-task> --region us-west-1
aws logs filter-log-events --log-group-name /ecs/job-syllabus --region us-west-1 \
  --start-time <ms-epoch-before-run-task> --filter-pattern "ingest run complete" --query 'events[].message' --output text
```

`aws ecs wait tasks-stopped` on a one-shot task is a real "did it actually
finish" signal in a way `run-task`'s own exit code isn't — it only confirms
the task was *submitted*, not that it *succeeded* (see phase-4.md bug #9,
found this exact way).

**Backfill a single company** without a full 49-company run — the
`backfill` Jenkins job wraps `INGEST_ONLY_COMPANY`, parameterized by
`COMPANY_SLUG` (must match a `slug` in `data/companies.yaml`). Trigger it
from the Jenkins UI ("Build with Parameters"), or run the equivalent
one-off task locally against real AWS:

```
DYNAMODB_ENDPOINT= S3_ENDPOINT= SQS_ENDPOINT= AWS_REGION=us-west-1 \
  RAW_BUCKET=<real bucket> EXTRACT_QUEUE_URL=$QUEUE_URL INGEST_ONLY_COMPANY=epic-games \
  go run ./cmd/ingest ingest
```

**CI jobs added this phase**: `worker-build`, `ingest-build`,
`rollup-build` (Kaniko build + Trivy scan + ECR push, same pattern as
`api-build`; `ingest-build`/`rollup-build` skip the ECS-service-deploy step
since those run as one-shot scheduled tasks, not long-lived services) and
`backfill` (parameterized, no build — just invokes `run-task` against the
already-built `ingest` image with `INGEST_ONLY_COMPANY` set). All four are
seeded by `ci/jobs.groovy`, visible in the same Jenkins instance as
section 6.
