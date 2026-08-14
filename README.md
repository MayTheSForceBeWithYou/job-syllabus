# Job Syllabus

Ingests game-industry job postings, extracts skill requirements, and ranks them by
frequency — turning the job market into a study syllabus for a Game DevOps /
build-release engineer career track.

See [docs/design.md](docs/design.md) for the full design and build plan. The project
is being built in phases per that document's §0 working agreement; each phase gets a
`docs/phase-N.md` writeup — [Phase 0](docs/phase-0.md) (scaffold),
[Phase 1](docs/phase-1.md) (local vertical slice),
[Phase 2](docs/phase-2.md) (Terraform baseline + Jenkins), and
[Phase 3](docs/phase-3.md) (deploy the read API) are all done.

**Status: Phase 0/1/2/3 complete.** `make ingest && make report` fetches real postings
from 5 companies (Epic Games, Riot Games, Discord, Roblox via Greenhouse; Kabam via
Lever), runs them through the extraction pipeline (normalize → segment → dictionary
match), and prints a ranked skill-frequency table — currently 43 distinct skills across
71 postings. The §6 hand-labeled validation gate passes at precision 1.000 / recall
0.942. See [PROGRESS.md](PROGRESS.md) for what's built, [NEXT_STEPS.md](NEXT_STEPS.md)
for what's next, and [HOW_TO_TEST.md](HOW_TO_TEST.md) to run it yourself.

Infrastructure runs for real on AWS (`us-west-1`): a Terraform-managed VPC, ECS cluster,
SQS queues, and a Jenkins controller at `jenkins.job-syllabus.skopekreep.com`, entirely
config-as-code (JCasC + Job DSL) with three seeded pipelines. See
[docs/phase-2.md](docs/phase-2.md) and [docs/bootstrap.md](docs/bootstrap.md).

The read-only `service-api` is deployed and live behind API Gateway, running on ECS
Fargate, serving real ranked-skill data from DynamoDB, built and deployed by Jenkins
from a git push (no auth yet — locked to the operator's IP). See
[docs/phase-3.md](docs/phase-3.md).

Not started: Stage 4 (Bedrock fallback), Stage 5 (review queue), auth (Phase 6's
Cognito JWT authorizer), scheduled/queued ingestion (`service-worker`), and the
Expo/mobile client.
