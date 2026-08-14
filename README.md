# Job Syllabus

Ingests game-industry job postings, extracts skill requirements, and ranks them by
frequency — turning the job market into a study syllabus for a Game DevOps /
build-release engineer career track.

See [docs/design.md](docs/design.md) for the full design and build plan. The project
is being built in phases per that document's §0 working agreement; each phase gets a
`docs/phase-N.md` writeup — [Phase 0](docs/phase-0.md) (scaffold) and
[Phase 1](docs/phase-1.md) (local vertical slice) are both done.

**Status: Phase 0/1 complete.** `make ingest && make report` fetches real postings from
5 companies (Epic Games, Riot Games, Discord, Roblox via Greenhouse; Kabam via Lever),
runs them through the extraction pipeline (normalize → segment → dictionary match), and
prints a ranked skill-frequency table — currently 41 distinct skills across 70
postings. The §6 hand-labeled validation gate passes at precision 1.000 / recall 0.942.
See [PROGRESS.md](PROGRESS.md) for what's built, [NEXT_STEPS.md](NEXT_STEPS.md)
for what's next, and [HOW_TO_TEST.md](HOW_TO_TEST.md) to run it yourself.

Not started: Stage 4 (Bedrock fallback), Stage 5 (review queue), and everything
explicitly out of scope for this phase — Terraform, Jenkins, auth, Expo/mobile client.
