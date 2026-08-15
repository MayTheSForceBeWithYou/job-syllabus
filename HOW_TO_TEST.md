# How to test what's built so far

This covers what actually works today. It'll grow as more of
`docs/design.md` gets implemented — check the date below against
`git log` if it's been a while.

**Last updated:** 2026-08-15 — Phase 0-6 complete, DoD and §6 validation
gate both passing, real AWS infrastructure + Jenkins + `service-api` +
queue-based ingestion + Bedrock fallback/review queue + Cognito auth +
Expo web client all live. Sections 1-5 below (local pipeline) are mostly
unchanged from Phase 1, but note section 3's update — `cmd/ingest` no
longer extracts inline (Phase 4), so seeing real skill data locally now
needs `make worker` running too, and `make worker` locally runs with
`BEDROCK_ENABLED=false` (Phase 5) since LocalStack can't emulate Bedrock.
Section 6 covers Phase 2's infrastructure, section 7 Phase 3's deployed
API, section 8 Phase 4's real ingestion pipeline, section 9 Phase 5's
review queue, section 10 (new) Phase 6's Cognito auth + Expo client.

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

## 9. Testing the review queue (Phase 5)

This needs the same AWS CLI credentials + `API_URL` as section 7.

**Watch unknown terms accumulate**: `cmd/worker` writes to the review
queue whenever a Bedrock finding doesn't match any known skill (yaml or
DynamoDB-approved). After a normal ingest+extract cycle:

```
curl "$API_URL/v1/reviews"
```

Sorted by occurrence count, each with a category guess and up to 5
example evidence spans. Empty is a legitimate result if nothing unmatched
has been seen recently — not a bug, just a quiet corpus.

**Triage a term** — three actions, `POST /v1/reviews/{term}`. The `{term}`
path segment is **base64url-encoded (no padding)**, not plain
URL-encoding — a term containing `/` (e.g. `CI/CD`) broke this route even
with correct `encodeURIComponent`, since API Gateway HTTP APIs
unconditionally decode `%2F` back into a literal `/` before forwarding the
path, splitting it into extra segments chi's single-segment `{term}` route
can't match (see `docs/phase-6.md`'s bug list). Encode with:

```
TERM=$(printf '%s' 'octane render' | base64 | tr '+/' '-_' | tr -d '=')
```

```
# Approve as a brand-new skill:
curl -X POST "$API_URL/v1/reviews/$(printf '%s' 'octane render' | base64 | tr '+/' '-_' | tr -d '=')" \
  -H 'Content-Type: application/json' -d '{"action":"create","category":"engines"}'

# Merge into an existing skill instead (e.g. a paraphrase of something already tracked):
curl -X POST "$API_URL/v1/reviews/$(printf '%s' 'helix core p4v plugin' | base64 | tr '+/' '-_' | tr -d '=')" \
  -H 'Content-Type: application/json' -d '{"action":"alias","mergeIntoSkillId":"perforce"}'

# Dismiss as noise — permanent, won't resurface even if the same phrase reappears:
curl -X POST "$API_URL/v1/reviews/$(printf '%s' 'team player' | base64 | tr '+/' '-_' | tr -d '=')" \
  -H 'Content-Type: application/json' -d '{"action":"reject"}'

# A term containing a slash — exactly the case that broke before this fix:
curl -X POST "$API_URL/v1/reviews/$(printf '%s' 'CI/CD' | base64 | tr '+/' '-_' | tr -d '=')" \
  -H 'Content-Type: application/json' -d '{"action":"reject"}'
```

Each of these was exercised locally against DynamoDB Local before ever
touching real AWS — seed a fake `REVIEW#PENDING` item directly (Bedrock is
disabled locally, so nothing populates the queue on its own):

```
aws dynamodb put-item --table-name jobsyllabus --endpoint-url http://localhost:8000 \
  --item '{"PK":{"S":"REVIEW#PENDING"},"SK":{"S":"TERM#octane render"},"entityType":{"S":"review"},"term":{"S":"octane render"},"category":{"S":"engines"},"count":{"N":"5"},"evidence":{"L":[]}}'
```

then run `cmd/api` locally (`DYNAMODB_ENDPOINT=http://localhost:8000 go run ./cmd/api`)
and hit its `/v1/reviews*` routes the same way.

**Confirm an approval actually reached the dictionary**: `GET
/v1/skills/{id}` should return the newly-created (or newly-aliased) skill
immediately — `RefreshSkills` runs synchronously as part of the
create/alias handler, not just on the 5-minute background tick, precisely
so this is observable right away:

```
curl "$API_URL/v1/skills/octane-render"
```

**Confirm re-extraction actually re-processes the corpus**: trigger the
`reextract` Jenkins job (manual, same `job-syllabus-rollup-reconcile` task
definition `backfill`/`rollup-build` already use, command overridden), or
run the equivalent task manually — same `aws scheduler get-schedule` /
`aws ecs run-task` pattern as section 8's manual triggers, but against the
`job-syllabus-rollup-reconcile` schedule name and a `"command":
["reextract"]` container override instead of an environment override.
Watch its own log line for the scanned/enqueued/skipped counts:

```
aws logs filter-log-events --log-group-name /ecs/job-syllabus --region us-west-1 \
  --filter-pattern "reextract:" --query 'events[].message' --output text
```

**Sanity-check Bedrock itself is reachable** (useful before blaming the
review queue for staying empty) — a direct `InvokeModel` call against the
same cross-region inference profile `internal/bedrock.ModelID` uses:

```
echo '{"anthropic_version":"bedrock-2023-05-31","max_tokens":50,"temperature":0,"messages":[{"role":"user","content":"Reply with exactly: OK"}]}' > /tmp/body.json
aws bedrock-runtime invoke-model --region us-west-2 \
  --model-id us.anthropic.claude-haiku-4-5-20251001-v1:0 \
  --content-type application/json --accept application/json \
  --body fileb:///tmp/body.json /tmp/out.json
cat /tmp/out.json
```

If this 403s from your own credentials, the worker's IAM role wouldn't be
your problem — check `aws bedrock get-inference-profile
--inference-profile-identifier us.anthropic.claude-haiku-4-5-20251001-v1:0
--region us-west-2` for the model's real routing regions and cross-check
`modules/service-worker/iam.tf`'s `task_bedrock` policy resources against
them (see `docs/phase-5.md` bugs #1-2 for why this isn't a bare
foundation-model ARN).

## 10. Testing Cognito auth + the Expo client (Phase 6)

**The deployed web client** is the real DoD target — open it in a browser:

```
WEB_URL=$(aws ssm get-parameter --name /job-syllabus/web/url --region us-west-1 --query Parameter.Value --output text)
echo "$WEB_URL"   # https://<distribution>.cloudfront.net
```

Sign in with an admin-created user (see below to create one). A working
sign-in redirects to Cognito's Hosted UI, back to the app with real tokens,
and all four tabs (Syllabus/Postings/Companies/Review) should show live
production data — the same numbers `curl`ing `$API_URL` directly returns.

**Create a Cognito user** — there's no public self-signup
(`allow_admin_create_user_only = true`), so users only exist via the AWS
CLI:

```
POOL_ID=$(aws ssm get-parameter --name /job-syllabus/auth/user_pool_id --region us-west-1 --query Parameter.Value --output text)
aws cognito-idp admin-create-user --user-pool-id "$POOL_ID" --username <email> --user-attributes Name=email,Value=<email> Name=email_verified,Value=true --region us-west-1
aws cognito-idp admin-set-user-password --user-pool-id "$POOL_ID" --username <email> --password '<a 12+ char password>' --permanent --region us-west-1
```

**Confirm the JWT authorizer itself, independent of the client** — a
request with no token should 401, a request with a garbage token should
also 401 (proving the authorizer actually validates the JWT, not just
checks presence):

```
curl -i "$API_URL/v1/skills"                              # {"message":"Unauthorized"}
curl -i "$API_URL/v1/skills" -H "Authorization: Bearer garbage"   # same
curl -i "$API_URL/healthz"                                 # 200, unauthenticated — unchanged from Phase 3
```

**Confirm CORS is actually configured** (bug #9 in `docs/phase-6.md` — this
silently breaks the web client even when the API itself is healthy):

```
curl -i -X OPTIONS "$API_URL/v1/skills" \
  -H "Origin: $WEB_URL" -H "Access-Control-Request-Method: GET" -H "Access-Control-Request-Headers: authorization"
# expect: HTTP/1.1 204, with access-control-allow-origin: $WEB_URL
```

**Run the mobile app locally** (Expo dev server, for iOS/Android code-path
review — this project does not claim device/simulator verification, see
`docs/phase-6.md`):

```
cd mobile
cp .env.example .env   # fill in the SSM-sourced values .env.example documents
npm install
npx expo start --web   # or --ios / --android if you have Xcode/Android Studio
```

**Rebuilding and redeploying the web client manually** (what
`client-build`'s "Export web build" → "Deploy to S3" → "Invalidate
CloudFront" stages do, useful for validating a change before waiting on
Jenkins):

```
cd mobile
cat > .env.production.local <<EOF
EXPO_PUBLIC_API_URL=$API_URL
EXPO_PUBLIC_COGNITO_DOMAIN=$(aws ssm get-parameter --name /job-syllabus/auth/hosted_ui_domain --region us-west-1 --query Parameter.Value --output text)
EXPO_PUBLIC_COGNITO_CLIENT_ID=$(aws ssm get-parameter --name /job-syllabus/auth/user_pool_client_id --region us-west-1 --query Parameter.Value --output text)
EXPO_PUBLIC_WEB_REDIRECT_URI=$WEB_URL
EOF
npx expo export -p web
BUCKET=$(aws ssm get-parameter --name /job-syllabus/web/bucket_name --region us-west-1 --query Parameter.Value --output text)
DIST_ID=$(aws ssm get-parameter --name /job-syllabus/web/distribution_id --region us-west-1 --query Parameter.Value --output text)
aws s3 sync dist/ "s3://$BUCKET/" --delete --region us-west-1
aws cloudfront create-invalidation --distribution-id "$DIST_ID" --paths "/*" --region us-west-1
```

**If the bundle throws `EXPO_PUBLIC_X is not set` in the browser console
despite the env file loading correctly** (bug #8 in `docs/phase-6.md`):
that's not an env-loading problem — grep the exported JS for the literal
expected value (e.g. the Cognito domain string). If it's absent, something
in `src/` is reading `process.env[someVariable]` (dynamic) instead of
`process.env.EXPO_PUBLIC_X` (static) — Expo's build-time inliner only
rewrites the static form.
