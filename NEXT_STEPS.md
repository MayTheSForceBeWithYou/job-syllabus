# Next steps

**Phase 0, Phase 1, and Phase 2 are all complete.** Phase 0/1 DoD met
(`docs/design.md` §17): `make ingest && make report` prints a ranked
skill-frequency table from real postings across 5 companies — see
PROGRESS.md for the actual output. The §6 validation gate also passes
(precision 1.000, recall 0.942, 70 hand-labeled postings). Phase 2 DoD met:
real Terraform-managed AWS infrastructure plus a fully config-as-code
Jenkins, validated by terminating the Jenkins instance and confirming
`terraform apply` alone brings it back fully working — see
[docs/phase-2.md](docs/phase-2.md). `docs/phase-0.md`, `docs/phase-1.md`,
and `docs/phase-2.md` writeups are all done (§0.6).

## Phase 3 and beyond (not started)

Per `docs/design.md`'s phase list: `service-api`/`service-worker` (the
actual application running on the ECS cluster Phase 2 built), auth
(Cognito), the API Gateway front door, and the Expo/mobile client. The
`api-build` Jenkins pipeline already has Deploy/Smoke-test stages written
but gated off (`when { expression { return false } }`) specifically until
`service-api` exists. `ci/jobs.groovy` also defers client-build, backfill,
and re-extract jobs to their own later phases, and `infra-plan`'s trigger
is push-based rather than true PR-discovery pending GitHub credentials in
Jenkins.

## What's below is carried over from Phase 1, still accurate

## Accepted: heading-keyword/structure classification doesn't fully generalize

Segmentation (Stage 2) classifies sections by matching heading text against
fixed keyword lists (`docs/design.md` §6); Stage 1 recognizes only real
`<h1>-<h6>` tags plus one bolded-paragraph pattern. This caught four real
gaps during this session — three fixed, one accepted:

- **Fixed**: Epic Games' "What we're looking for" (requirements), Riot's
  "Desired Qualifications" (nice_to_have), and Roblox's structural pattern
  of using `<p><strong>Heading</strong></p>` instead of real heading tags
  at all. All three were common, well-defined patterns from major seed
  companies silently zeroing out real extraction — see
  `internal/extract/classify.go` and `internal/extract/normalize.go`.
- **Accepted, not fixed**: Kabam's Lever postings use free-form headings
  ("In this role, you can expect to:") that match nothing and default to
  boilerplate, contributing zero skills — and separately, some Kabam
  postings are in French, which no English cue-list tuning addresses at
  all. Decided 2026-08-13 not to chase either with a fallback heuristic,
  endless cue-list whack-a-mole, or i18n support for a 5-company slice —
  see the KNOWN GAP comment on `classifyHeading` and
  `RequirementSections`. The validation set (`testdata/labeled/`) reflects
  this honestly: one Kabam posting was hand-labeled from its true raw
  content specifically so the gap shows up as a measured recall hit
  (0.942, not 1.000) instead of being defined away. Revisit once the
  review queue (§6 Stage 5) exists, or if scaling past 5 companies shows
  this costing a meaningful chunk of the corpus.

The distinction that matters going forward: a heading gap from a **major
company already in `companies.yaml`** is worth fixing immediately (it's
silently deleting real data from the DoD's own numbers); a heading gap
from a **one-off company** is not worth chasing indefinitely. Check
`report`'s per-skill numbers and `ingest`'s per-company `skillEdges` count
in the log — a company at 0 while everything else is non-zero is the
signal, exactly how the Epic bug was found.

## Deliberate Phase 1 simplification: no write-time counters

`docs/design.md` §4 specifies write-time `TransactWriteItems` counters
with idempotent conditional writes, reconciled nightly by `cmd/rollup`,
to survive an at-least-once SQS consumer double-processing a posting.
Phase 1's `cmd/ingest` is single-process with no queue, so that failure
mode doesn't exist yet — `report` just recomputes counts from a fresh
scan of all `PostingSkill` edges every run (`internal/store/skills.go`).
Correct by construction, can't drift, but doesn't scale past a
few-hundred-item corpus and isn't what §4 describes. **Build the real
mechanism when `cmd/worker` becomes a queue consumer** (Phase 4+ per the
reference doc's phase list) — don't let this simplification quietly
become permanent by inertia.

## Ordered task list

1. **Expand `data/skills.yaml`** toward the 250-400 skill target — ~90
   seeded now, real coverage gaps already visible in the hand-labeling
   review (Airflow, Spark, Databricks, Kinesis named in §6's own taxonomy
   table but not yet seeded; OAuth/OIDC, PyTorch/TensorFlow, and other
   terms seen in real postings but out of dictionary scope).
2. **Company registry growth**: research more real Greenhouse/Lever/Ashby/
   Workday tokens per §5 — 5 companies is enough to prove the pipeline,
   not enough for a meaningful "which skills matter" conclusion (that's
   the actual point of the project per §1). More companies also means
   more hand-labeled postings to keep the validation set representative.
3. Everything in "Explicitly still out of scope" below, in whatever order
   the operator picks when ready to move past Phase 1.

## Explicitly still out of scope

Terraform, Jenkins/JCasC, Cognito/auth, Expo/mobile client, Bedrock
fallback (§6 Stage 4), review queue (§6 Stage 5), role-family
classification (every posting is currently `RoleFamily: unclassified`),
`meta`-category structured-fact extraction (years of experience, degree,
shipped-title). All per `docs/design.md` §17 and the operator's explicit
session scope.
