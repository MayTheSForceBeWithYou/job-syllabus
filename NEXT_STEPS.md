# Next steps

**Phase 0 through Phase 5 are all complete.** Phase 0/1 DoD met
(`docs/design.md` §17): `make ingest && make report` prints a ranked
skill-frequency table from real postings — see PROGRESS.md for the actual
output. The §6 validation gate also passes (precision 1.000, recall
0.942, 70 hand-labeled postings). Phase 2 DoD met: real Terraform-managed
AWS infrastructure plus a fully config-as-code Jenkins, validated by
terminating the Jenkins instance and confirming `terraform apply` alone
brings it back fully working — see [docs/phase-2.md](docs/phase-2.md).
Phase 3 DoD met: `service-api` is deployed on ECS Fargate behind API
Gateway, serving real ranked-skill data, built and deployed by Jenkins
from a git push — see [docs/phase-3.md](docs/phase-3.md). Phase 4 DoD
mostly met: all six Tier-1 connectors, a 49-company registry, a real
SQS-queued `cmd/worker`, idempotent counters, and daily EventBridge
scheduling are all live in production; the one line item short is 500+
postings (real number 158) — an honest, explained real-world-volume
finding, not a bug — see [docs/phase-4.md](docs/phase-4.md). Phase 5 DoD
met: unknown terms surface in `GET /v1/reviews`, and approving one via the
API updates the live (DynamoDB-backed) dictionary immediately, with
`cmd/rollup reextract` picking it up across the existing corpus — see
[docs/phase-5.md](docs/phase-5.md).
`docs/phase-0.md` through `docs/phase-5.md` writeups are all done (§0.6).

## Phase 6 and beyond (not started)

Per `docs/design.md`'s phase list: auth (Cognito JWT authorizer —
`service-api` is locked to the operator's IP by an application-layer
allowlist in the meantime, see `internal/api/ipallow.go`), role-family
classification (every posting is still `RoleFamily: unclassified`, so
`STAT#` counters currently only ever have one bucket), and the Expo/mobile
client. `infra-plan`'s trigger is push-based rather than true PR-discovery
pending GitHub credentials in Jenkins — the same missing-credential gap
Phase 5 hit for `data/skills.yaml` writeback (see docs/phase-5.md); worth
solving once, for both, if this project ever needs a GitHub App/PAT.
Stage 4's escalation-to-Sonnet path (docs/design.md §6: "escalate to
Sonnet if precision on the validation set is short of target") hasn't
been needed — Haiku alone has kept the §6 gate passing.

## Keeping `data/skills.yaml` in sync with DynamoDB's live dictionary

Phase 5's review-queue approvals write straight to DynamoDB, not to the
git-tracked `data/skills.yaml` seed (no GitHub write credential exists in
this project — see docs/phase-5.md's "DynamoDB instead of a git commit").
That means the two drift apart over time: DynamoDB is always the live,
authoritative dictionary, but `data/skills.yaml` only reflects whatever
was true at the last manual sync. Not urgent (a fresh deploy still works
correctly — it just starts with fewer approved skills until the next
`RefreshSkills` reload), but worth a small `cmd/rollup export-skills` (or
similar) that dumps DynamoDB's canonical skills as yaml, to diff by hand
against `data/skills.yaml` periodically rather than letting the drift
become invisible.

## Growing the company registry toward 500+ postings

Phase 4's honest finding (see `docs/phase-4.md`): real-world role-matched
volume runs closer to 3-4 postings per company than the ~12/company the
"500+" target implicitly assumed, even across a diversified,
individually-verified 49-company registry. Two real, non-mutually-
exclusive paths forward, neither of which involves loosening
`roleFilters` until off-thesis roles start counting:
- **Add more companies.** `data/companies.yaml`'s header documents the
  verification discipline that matters here — HTTP 200 alone isn't
  enough; always cross-check the response's own `company_name`/job-title
  fields against the intended company (two tokens looked right and
  weren't, see `docs/phase-4.md` bug #2). Budget real time for this, not
  a quick pass — the useful hit rate drops fast past the ~50 most
  obvious candidates.
- **Let the daily schedule accumulate.** This is what EventBridge's daily
  06:00 UTC `cmd/ingest` run is actually for — the corpus grows over
  weeks as different roles open/close across the registry, not just from
  a single ingest run's snapshot.

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
