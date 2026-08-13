# Next steps

**Current DoD** (`docs/design.md` §17, operator-confirmed scope for this
session): `make ingest && make report` prints a ranked skill-frequency
table from real postings across ≥5 companies. Ingest side is done and
verified (see PROGRESS.md). Report is still the interim posting-count
version — the ranked table needs Stage 3.

## Open decision: heading-keyword classification doesn't generalize

Segmentation (Stage 2) classifies sections by matching their heading text
against fixed keyword lists (`docs/design.md` §6: "requirements",
"qualifications", "nice to have", "bonus", etc.). This works well for
Greenhouse postings tested so far (Epic Games, Riot Games) but **fails
completely** on at least one real Lever posting: Kabam's "Associate Live
Operations Specialist" uses `"In this role, you can expect to:"` and `"To
be successful in this role, your background includes:"` as its section
headings — neither matches any cue, so both sections default to
`boilerplate` and this posting would contribute **zero** extracted skills
to Stage 3, despite having real content (Jira, Google Workspace, etc.) in
the second section.

This isn't a one-off typo to patch (like the "desired qualifications" fix
already made) — it's evidence that pure heading-keyword matching won't
generalize across the ~40 companies §15 asks for, especially indie/midsize
studios that don't use standard HR-template language. Options, roughly in
order of effort:

1. **Accept it as a known gap for now.** Postings with non-standard
   headings just contribute fewer skills; the review queue (§6 Stage 5,
   not built yet) is the long-term fix once Bedrock fallback exists —
   novel terms surface there regardless of which section they came from.
2. **Add a fallback heuristic**: if a posting has zero requirements/
   nice_to_have sections after heading classification, treat the last
   substantive non-boilerplate section (or any section with enough bullet
   lines) as a lower-confidence requirements candidate.
3. **Widen the cue lists opportunistically** as more real postings get
   tested — same pattern as the "desired qualifications" fix, but this is
   whack-a-mole against arbitrary phrasing and won't fully close the gap.

Needs an operator decision before Stage 3 locks in what "requirements
content" means. Not resolved as of `9c0dd83`.

## Ordered task list to close the DoD gap

1. **Resolve the heading-classification decision above.**
2. **`data/skills.yaml`** — canonical skills + aliases covering the
   categories in §6's taxonomy table. Needs its own quick schema check
   before writing (same pattern as the two prior checkpoints) since it's
   the last piece before Stage 3 can run.
3. **Stage 3 (dictionary matching)** in `internal/extract`: word-boundary/
   regex matching of `skills.yaml` aliases against `RequirementSections`
   output, producing `PostingSkill` edges with `Required`/`Confidence`/
   `Method="dict"`.
4. **Wire Stage 1-3 into `cmd/ingest`**: run extraction on newly-ingested
   (or `ExtractVer < current`) postings, write `PostingSkill` edges via
   `TransactWriteItems` with the idempotent counter-increment pattern from
   §4 ("The aggregation problem").
5. **Rewrite `report`** to produce the actual ranked skill-frequency table
   (skill, count, % of postings, required vs. nice-to-have split) instead
   of the interim posting-count view.
6. **Validate the DoD**: `make ingest && make report` end to end against
   the 5 seed companies (or more — §5 budgets a couple hours to find more
   real Greenhouse/Lever/Ashby tokens if 5 turns out too thin for a
   meaningful ranking).
7. **`docs/phase-0.md` + `docs/phase-1.md` writeups** (§0.6 — required, not
   optional). Should cover: the design-doc source-of-truth conflict and
   how it was resolved, the DynamoDB Local hang postmortem, the two
   extraction bugs only found via live-data testing, and this heading-
   classification open question.

## Explicitly still out of scope

Terraform, Jenkins/JCasC, Cognito/auth, Expo/mobile client, Bedrock
fallback (§6 Stage 4), review queue (§6 Stage 5), hand-labeled precision
gate (§6 "Validation" — 50 postings, 90% precision threshold). All per
`docs/design.md` §17 and the operator's explicit session scope.
