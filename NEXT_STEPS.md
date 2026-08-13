# Next steps

**Current DoD** (`docs/design.md` §17, operator-confirmed scope for this
session): `make ingest && make report` prints a ranked skill-frequency
table from real postings across ≥5 companies. Ingest side is done and
verified (see PROGRESS.md). Report is still the interim posting-count
version — the ranked table needs Stage 3.

## Resolved: heading-keyword classification doesn't generalize (accepted, not fixed)

Segmentation (Stage 2) classifies sections by matching their heading text
against fixed keyword lists (`docs/design.md` §6: "requirements",
"qualifications", "nice to have", "bonus", etc.). This works well for
Greenhouse postings tested so far (Epic Games, Riot Games) but **fails
completely** on at least one real Lever posting: Kabam's "Associate Live
Operations Specialist" uses `"In this role, you can expect to:"` and `"To
be successful in this role, your background includes:"` as its section
headings — neither matches any cue, so both sections default to
`boilerplate` and this posting contributes **zero** extracted skills to
Stage 3, despite having real content (Jira, Google Workspace, etc.) in the
second section.

**Decision (2026-08-13): accept the gap for now.** Postings with
non-standard headings just contribute fewer skills; not worth chasing
arbitrary phrasing or adding a confidence-scored fallback for a Phase 1
local slice covering 5 companies. Documented in code at
`internal/extract/classify.go` (`classifyHeading`'s KNOWN GAP comment) and
`internal/extract/section.go` (`RequirementSections`) so it reads as an
accepted limitation, not a bug, if rediscovered later. Revisit once the
review queue (§6 Stage 5) exists — that surfaces novel terms independent
of which section they came from, which is the more durable fix — or if
scaling past the 5 seed companies shows this costing a meaningful chunk of
the corpus.

## Ordered task list to close the DoD gap

1. **`data/skills.yaml`** — canonical skills + aliases covering the
   categories in §6's taxonomy table. Needs its own quick schema check
   before writing (same pattern as the two prior checkpoints) since it's
   the last piece before Stage 3 can run.
2. **Stage 3 (dictionary matching)** in `internal/extract`: word-boundary/
   regex matching of `skills.yaml` aliases against `RequirementSections`
   output, producing `PostingSkill` edges with `Required`/`Confidence`/
   `Method="dict"`.
3. **Wire Stage 1-3 into `cmd/ingest`**: run extraction on newly-ingested
   (or `ExtractVer < current`) postings, write `PostingSkill` edges via
   `TransactWriteItems` with the idempotent counter-increment pattern from
   §4 ("The aggregation problem").
4. **Rewrite `report`** to produce the actual ranked skill-frequency table
   (skill, count, % of postings, required vs. nice-to-have split) instead
   of the interim posting-count view.
5. **Validate the DoD**: `make ingest && make report` end to end against
   the 5 seed companies (or more — §5 budgets a couple hours to find more
   real Greenhouse/Lever/Ashby tokens if 5 turns out too thin for a
   meaningful ranking).
6. **`docs/phase-0.md` + `docs/phase-1.md` writeups** (§0.6 — required, not
   optional). Should cover: the design-doc source-of-truth conflict and
   how it was resolved, the DynamoDB Local hang postmortem, the two
   extraction bugs only found via live-data testing, and the accepted
   heading-classification gap.

## Explicitly still out of scope

Terraform, Jenkins/JCasC, Cognito/auth, Expo/mobile client, Bedrock
fallback (§6 Stage 4), review queue (§6 Stage 5), hand-labeled precision
gate (§6 "Validation" — 50 postings, 90% precision threshold). All per
`docs/design.md` §17 and the operator's explicit session scope.
