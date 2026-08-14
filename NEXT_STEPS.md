# Next steps

**DoD met** (`docs/design.md` §17): `make ingest && make report` prints a
ranked skill-frequency table from real postings across 5 companies — see
PROGRESS.md for the actual output. Phase 0/1 (as scoped for this session)
is functionally done; what's left is the required writeups and some
known gaps that don't block the DoD but matter before treating this as
durable.

## Accepted: heading-keyword classification doesn't fully generalize

Segmentation (Stage 2) classifies sections by matching heading text
against fixed keyword lists (`docs/design.md` §6). This caught two real
gaps during this session — one fixed, one accepted:

- **Fixed**: Epic Games' "What we're looking for" (requirements) and
  Riot's "Desired Qualifications" (nice_to_have) weren't in the original
  §6 cue list. Both are common, well-defined phrases from major seed
  companies, so both got added directly — see `internal/extract/classify.go`.
- **Accepted, not fixed**: Kabam's Lever postings use free-form headings
  ("In this role, you can expect to:") that match nothing and default to
  boilerplate, contributing zero skills. Decided 2026-08-13 not to chase
  this with a fallback heuristic or endless cue-list whack-a-mole — see
  the KNOWN GAP comment on `classifyHeading` and `RequirementSections`.
  Revisit once the review queue (§6 Stage 5) exists, or if scaling past 5
  companies shows this costing a meaningful chunk of the corpus.

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

1. **`docs/phase-0.md` + `docs/phase-1.md` writeups** (§0.6 — required,
   not optional, and now unblocked since the DoD passes). Should cover:
   the design-doc source-of-truth conflict and its resolution, the
   DynamoDB Local hang postmortem, the three extraction bugs found via
   live-data testing (one general lesson: check per-company/per-skill
   numbers, not just "did it run"), the accepted Kabam gap, and the
   deliberate counter-mechanism deferral above.
2. **Hand-labeled validation set** (§6 "Validation"): label 50 postings
   into `testdata/labeled/*.json`, add a `go test ./internal/extract`
   precision/recall gate at 90%. Not done — the ranked table above is
   plausible-looking but has no measured precision number backing it.
3. **Expand `data/skills.yaml`** toward the 250-400 skill target — ~90
   seeded now, real coverage gaps will surface as more companies get
   added (§15 budgets research time for ~40 total).
4. **Company registry growth**: research more real Greenhouse/Lever/Ashby/
   Workday tokens per §5 — 5 companies is enough to prove the pipeline,
   not enough for a meaningful "which skills matter" conclusion (that's
   the actual point of the project per §1).
5. Everything in "Explicitly still out of scope" below, in whatever order
   the operator picks when ready to move past Phase 1.

## Explicitly still out of scope

Terraform, Jenkins/JCasC, Cognito/auth, Expo/mobile client, Bedrock
fallback (§6 Stage 4), review queue (§6 Stage 5), role-family
classification (every posting is currently `RoleFamily: unclassified`),
`meta`-category structured-fact extraction (years of experience, degree,
shipped-title). All per `docs/design.md` §17 and the operator's explicit
session scope.
