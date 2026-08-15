# Job Syllabus

Ingests game-industry job postings, extracts skill requirements, and ranks them by
frequency — turning the job market into a study syllabus for a Game DevOps /
build-release engineer career track.

See [docs/design.md](docs/design.md) for the full design and build plan. The project
is being built in phases per that document's §0 working agreement; each phase gets a
`docs/phase-N.md` writeup — [Phase 0](docs/phase-0.md) (scaffold),
[Phase 1](docs/phase-1.md) (local vertical slice),
[Phase 2](docs/phase-2.md) (Terraform baseline + Jenkins),
[Phase 3](docs/phase-3.md) (deploy the read API),
[Phase 4](docs/phase-4.md) (ingestion at scale),
[Phase 5](docs/phase-5.md) (Bedrock + review queue), and
[Phase 6](docs/phase-6.md) (Cognito auth + Expo client) are all done.

**Status: Phase 0-6 complete.** `make ingest && make report` fetches real postings from
49 verified companies (Epic Games, Riot Games, Nintendo, Krafton, and 45 others across
Greenhouse/Lever/Ashby/SmartRecruiters/Workable/Workday), runs them through the
extraction pipeline (normalize → segment → dictionary match → Bedrock fallback), and
prints a ranked skill-frequency table. The §6 hand-labeled validation gate passes at
precision 1.000 / recall 0.942. See [PROGRESS.md](PROGRESS.md) for what's built,
[NEXT_STEPS.md](NEXT_STEPS.md) for what's next, and [HOW_TO_TEST.md](HOW_TO_TEST.md)
to run it yourself.

Infrastructure runs for real on AWS (`us-west-1`): a Terraform-managed VPC, ECS cluster,
SQS queues, and a Jenkins controller at `jenkins.job-syllabus.skopekreep.com`, entirely
config-as-code (JCasC + Job DSL) with eight seeded pipelines. See
[docs/phase-2.md](docs/phase-2.md) and [docs/bootstrap.md](docs/bootstrap.md).

The read-only `service-api` is deployed and live behind API Gateway, running on ECS
Fargate, serving real ranked-skill data from DynamoDB, built and deployed by Jenkins
from a git push (no auth yet — locked to the operator's IP). See
[docs/phase-3.md](docs/phase-3.md).

Ingestion runs at scale: `cmd/ingest` fetches from all six Tier-1 ATS platforms daily
(EventBridge-scheduled) and hands each posting to `cmd/worker` via a real SQS queue for
extraction, with idempotent write-time skill counters and a nightly `cmd/rollup
reconcile` pass that corrects drift. See [docs/phase-4.md](docs/phase-4.md).

Unmatched requirement bullets fall back to Claude Haiku (Amazon Bedrock, cross-region
inference profile in `us-west-2`), cached by bullet-text hash; anything Bedrock finds
that isn't already a known skill surfaces in a review queue (`GET /v1/reviews`) for
triage (`POST /v1/reviews/{term}`: create/alias/reject). Approved skills write straight
to DynamoDB — the live source of truth `cmd/api`/`cmd/worker` both merge in on a
5-minute refresh — and `cmd/rollup reextract` re-runs the corpus after a dictionary
version bump so approvals reach already-processed postings. See
[docs/phase-5.md](docs/phase-5.md).

A real Cognito User Pool (admin-created users only) now fronts `service-api` via an API
Gateway JWT authorizer, replacing Phase 3's IP allowlist — the read/admin OAuth scopes
map directly onto GET/POST. An Expo Router client (sign-in, ranked syllabus, postings,
companies, review-queue triage) is deployed as a static web build behind S3 +
CloudFront, verified live end-to-end against real production data; iOS/Android are
verified by code review against current SDK docs, not by running on a device or
simulator (no Xcode/Android Studio available in this environment). See
[docs/phase-6.md](docs/phase-6.md).

Not started: Stage 4's escalation-to-Sonnet path (not needed yet — Haiku alone has kept
precision on target), and actual iOS/Android device or simulator verification.
