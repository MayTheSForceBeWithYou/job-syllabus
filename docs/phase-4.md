# Phase 4 — Ingestion at scale

**Status: done. DoD mostly met — one honest, explained shortfall.** All
six Tier-1 ATS connectors exist and are wired up; the company registry
grew from 5 to 49 real, individually-verified companies; a real SQS queue
now sits between `cmd/ingest` and `cmd/worker`, with idempotent
write-time counters and a `cmd/rollup reconcile` pass that recounts and
corrects drift; daily EventBridge-scheduled ingestion and reconciliation
are live in production; DLQs are empty. The one DoD line item not met is
**500+ postings** — the real number is 158, for reasons explained below
that are about real-world hiring volume, not a bug.

```
$ curl https://o79zaeqqna.execute-api.us-west-1.amazonaws.com/prod/v1/stats/overview
{"postingCount":158,"companyCount":20,"skillEdgeCount":370,
 "distinctSkillsMatched":55,"coveragePct":54.4}
```

## What was built

**All six Tier-1 connectors** (`internal/connectors/`): Ashby,
SmartRecruiters, Workable, and Workday joined the existing Greenhouse and
Lever. Every one was verified against a real, reachable board before
being trusted — not just "does it compile": Ashby against `linear`,
SmartRecruiters' two-call list+detail shape against `visa`, Workable's
undocumented `?details=true` single-call variant against `ripple`, and
Workday's paginated list + per-posting detail against a real tenant
(`citi.wd5`), which is also how Workday's 20-item server-side page-size
cap and its need for more per-company identity than a single `Token`
field (solved by packing `{subdomain}/{site}` into it) got discovered.
Lever gained an `"eu:"` token prefix for EU-hosted accounts
(`api.eu.lever.co`) after Frontier Developments turned out to need it.

**Company registry, 5 → 49**, every entry verified live with its
`company_name` field cross-checked against the intended company (two
plausible-looking tokens were caught and dropped for resolving to
unrelated companies that happen to share a short board name — see "What
broke"). `roleFilters` broadened based on empirical review of real
unmatched titles across the larger registry (`sre`, `site reliability`,
`pipeline`, `automation`, `tools`, `developer experience`).

**Deduplication and lifecycle** (`internal/store/dedup.go`,
`cmd/ingest`): a `DEDUP#<contentHash>` marker, claimed via a conditional
`PutItem`, collapses the same job cross-posted under two URLs into one
posting — backed by DynamoDB TTL (30 days) so the marker doesn't need a
cleanup job. A posting that disappears from a company's feed gets
`ClosedAt` stamped rather than deleted, and closed postings are now
excluded from every "active" stat (`GET /v1/skills`, `/v1/stats/overview`,
`report`) while staying in the table for historical trend, per
`docs/design.md` §4.

**A real SQS queue boundary** (`internal/queue`, `internal/rawstore`,
`cmd/worker`): `cmd/ingest` no longer extracts inline. It uploads each
posting's raw connector output to S3 (`internal/rawstore` — DynamoDB has
no body field; `model.Posting.RawS3Key` had been anticipating this since
Phase 2) and enqueues a reference. `cmd/worker`, previously a panic stub,
is now a real long-polling SQS consumer that loads the posting + raw
content, runs Stages 1-3 extraction, writes edges, and deletes the
message — graceful SIGTERM shutdown matching `cmd/api`'s pattern.

**Idempotent write-time counters** (`internal/store.PutSkillEdge`, "the
aggregation problem" per `docs/design.md` §4): a `TransactWriteItems` call
writes the `PostingSkill` edge (`ConditionExpression:
attribute_not_exists(PK)`) and `ADD`-increments a
`STAT#<roleFamily>/SKILL#<sid>` counter in the same transaction. An
at-least-once SQS redelivery hitting the edge's condition failure is
treated as a successful no-op, not an error — confirmed by deliberately
re-running the same batch through ingest+worker twice and checking every
counter and edge count stayed exactly the same. `cmd/rollup reconcile`
recounts every counter from the real edges (excluding closed postings)
and corrects drift; confirmed both that it no-ops on an already-correct
store and that it detects+fixes a deliberately-injected bad value — and,
in production, it found and corrected four counters that had genuinely
drifted from earlier testing.

**Terraform**: `modules/service-worker` (Fargate Spot, min_capacity=0,
queue-depth step-scaling via two CloudWatch alarms on
`ApproximateNumberOfMessagesVisible` — no ALB, nothing calls into it) and
`modules/task-scheduled` (a reusable EventBridge Scheduler → `ecs:RunTask`
module, instantiated for `cmd/ingest` daily 06:00 UTC and `cmd/rollup
reconcile` daily 07:00 UTC). Both reuse existing resources rather than
creating new ones — the general-purpose `fargate` security group, and the
`raw/` S3 prefix + lifecycle rule `modules/data/s3.tf` had been waiting
for since Phase 2.

**Jenkins**: `worker-build`, `ingest-build`, and `rollup-build` pipelines
(discovered mid-phase that the latter two didn't exist at all — see "What
broke" #10), plus a parameterized `backfill` job for on-demand
single-company re-ingestion, backed by a new `INGEST_ONLY_COMPANY` filter
in `cmd/ingest` itself rather than hand-sliced YAML in the Jenkinsfile.

## What broke

Same discipline as every prior phase: real bugs found by running the
actual thing, not by re-reading the code. In encountered order:

1. **Workday's list endpoint rejects `limit` above 20.** A real tenant
   returned a bare `HTTP_400` with no explanatory message for `limit: 50`;
   confirmed the actual cap by bisecting (10 worked, 25 didn't) rather
   than guessing from documentation Workday doesn't publish.
2. **Two Greenhouse tokens resolved to the wrong company despite HTTP
   200 and real postings.** `bethesda` → a physical-therapy practice;
   `peak` → a fitness/wellness company. Both were caught only because
   every candidate's `company_name` field (present in Greenhouse's own
   response) was cross-checked against the intended company before being
   trusted — a real, live instance of the same "silent data-quality bug"
   class as Phase 1's Epic/Roblox heading gaps, just one layer further
   upstream (wrong company, not wrong extraction).
3. **162 role-matched postings from a 41-company registry, not remotely
   close to 500.** Broadening `roleFilters` based on empirical review of
   real unmatched titles (`sre`, `pipeline`, `tools`, etc.) helped some
   (123 → 161) but didn't close the gap — genuinely scarce real-time
   volume, not a matching bug (see "What would be done differently").
4. **`api.lever.co` 404s outright for a real company's correct token.**
   Frontier Developments' public career site lives at
   `jobs.eu.lever.co`; its API host is `api.eu.lever.co`, a real, if
   uncommon, Lever region configuration — not documented anywhere
   obvious, found by trying the EU-domain variant after the non-EU one
   failed.
5. **`ClaimContentHash` would have misfired on every normal re-ingest**
   if applied unconditionally — an already-known posting's re-ingest
   would find its own marker (created the first time it was seen) and
   incorrectly treat itself as a cross-posted duplicate. Caught before
   shipping by tracing the logic, not by a live failure: the fix scopes
   the dedup check to only genuinely-new posting IDs.
6. **The Jenkins CI role can create/tag IAM roles and inline role
   policies, but not standalone managed policies.** A real
   `infra-apply` run failed with `AccessDenied` on `iam:TagPolicy` for
   three `aws_iam_policy` resources — everything else in the same apply
   (roles, ECS service, autoscaling, CloudWatch alarms, both scheduled
   tasks) succeeded. Fixed by switching `service-worker`'s policies to
   inline (`aws_iam_role_policy`), the same pattern
   `iam_scheduled_tasks.tf` had already used successfully in the identical
   apply, rather than widening the CI role's own IAM permissions.
7. **Trivy caught 5 real HIGH-severity CVEs** in `golang.org/x/net`
   (v0.52.0, several releases behind) on `worker-build`'s first-ever scan
   — the security gate doing exactly its job. Bumped to v0.58.0, which
   applies to every binary in the repo (single `go.mod`); re-ran
   `api-build` too so the already-deployed API picked up the same fix.
8. **`worker-build`'s smoke test failed even though the deploy genuinely
   worked.** Direct inspection of the container's own CloudWatch log
   (`aws logs get-log-events`) showed `"worker: starting" skills=89` —
   real success — while the pipeline reported "never saw that log line."
   Root cause: `START_TIME` was computed as "now − 60s" only *after*
   `aws ecs wait services-stable` returned, and that wait took ~4 minutes
   for cold Fargate start; the actual log line was written mid-wait, well
   before the search window even began. Fixed by capturing `START_TIME`
   before `update-service` is even called, not after the wait.
9. **The real EventBridge-scheduled ingest task failed outright:
   `CannotPullContainerError`, `:latest` not found.** There was no build
   pipeline for `cmd/ingest` or `cmd/rollup` at all — Terraform's
   bootstrap task definition only ever pointed at a placeholder tag
   nothing had pushed, an oversight that only surfaced by actually
   invoking the real scheduled task via `run-task` rather than trusting
   `terraform apply`'s exit code. Fixed by adding
   `ci/Jenkinsfile.ingest-build`/`rollup-build`, and — since a
   Jenkins-registered new revision needs somewhere to take effect —
   repointing EventBridge Scheduler's target at the task definition
   *family* (revision-less ARN) instead of the specific revision
   Terraform's bootstrap apply created, the same "Jenkins owns the live
   revision" pattern the ECS services already use via `ignore_changes`,
   expressed differently since Scheduler has no `update-service`
   equivalent.
10. **The worker's own task role was missing `dynamodb:UpdateItem`.**
    A real worker run against the actual production queue failed every
    single skill-edge write with `AccessDeniedException` — confirmed
    directly via CloudWatch Logs. Root cause: IAM evaluates
    `TransactWriteItems` per-item against the single-item action matching
    each `TransactItem`'s own operation, not just against
    `TransactWriteItems` itself — the edge write is a `Put` (already
    granted), but the `STAT#` counter increment (the other half of the
    same transaction) is an `Update`, which needs its own grant that had
    simply been left off the policy when it was written.

## What would be done differently

- **"500+ postings" was calibrated against an assumption that didn't
  hold up against real data**: that a diversified ~40-company registry
  would yield roughly a dozen role-matched postings per company. The
  real, measured rate across 49 verified companies is closer to 3-4.
  Reaching 500 at that rate needs something like 150 companies, not 40 —
  a scale of manual verification effort ("check `company_name`, not just
  HTTP 200") that wasn't accounted for going in. The honest fix isn't to
  loosen `roleFilters` until irrelevant roles start counting (that would
  quietly break the whole point of the project — a build/release/DevOps-
  specific study corpus, not "any software job") — it's either accepting
  a lower initial number and letting the daily schedule accumulate it
  over weeks (which is what the schedule is actually for), or budgeting
  real time for a much larger company-verification pass in a follow-up
  session.
- **Bugs #9 and #10 (no ingest/rollup build pipeline; missing
  `UpdateItem`) both would have been caught by treating "the code
  compiles and `terraform apply` exits 0" as exactly what it is — a
  necessary, not sufficient, check** — and by actually invoking the real
  scheduled task end-to-end (`run-task`, watch the logs) before calling
  the infrastructure done. This is the same lesson as every prior phase's
  writeup, restated because it keeps paying off: `aws ecs wait
  tasks-stopped` + `aws logs get-log-events` found two genuine, otherwise
  invisible production bugs in about five minutes each.
- **IAM least-privilege scoping (#10) needs the same "verify against
  real behavior" discipline as everything else** — reasoning about which
  DynamoDB actions a `TransactWriteItems` call needs from first
  principles (Put→PutItem, seemed complete) missed that `Update`→
  `UpdateItem` is a *separate* grant IAM checks per-item, not implied by
  granting `TransactWriteItems` itself. Worth explicitly listing every
  distinct operation type used inside a transaction next time, rather
  than pattern-matching from a similar-looking existing policy.
- **Building or testing against a genuinely fresh environment early
  would have caught #2 (wrong-company tokens) and #6 (IAM tagging gap)
  faster** — both were artifacts of assumptions (that an HTTP 200 with
  real-looking data means the right company; that a scoped CI role's
  permissions generalize across different IAM resource types) that only
  broke under real, previously-unexercised conditions.
