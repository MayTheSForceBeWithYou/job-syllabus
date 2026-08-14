# Phase 1 — Local vertical slice

**Status: done. DoD passes.** `make ingest && make report` prints a ranked
skill-frequency table from real postings across 5 companies (`docs/design.md`
§17). See [phase-0.md](phase-0.md) for the scaffold/tooling half of this
session; this covers the actual vertical slice — connectors through
extraction through the validation gate.

## What was built

**Connectors** (`internal/connectors`): `Connector` interface reviewed with
the operator before implementation (checkpoint 1), `GreenhouseConnector` and
`LeverConnector` against their real public JSON APIs, registry keyed by ATS
name. Hardened after a production-shaped incident (see below): explicit
dial/TLS-handshake/response-header sub-timeouts, `FetchWithRetry` (2 retries,
exponential backoff), `slog` structured logging around every fetch.

**Store** (`internal/store`): single-table DynamoDB item shapes from §4
(`Posting`, `PostingSkill`), `EnsureTable` for idempotent local table
creation, `UpsertPosting` for the dedup rule from §5.

**Extraction pipeline** (`internal/extract`), Stages 1-3 of the 5 in §6,
reviewed with the operator before implementation (checkpoint 2):
1. Normalize — `htmlToLines` (goquery): strips nav/footer/script/style,
   flattens to `"## heading"` / `"- item"` lines.
2. Segment — splits on heading markers, classifies against ordered cue lists
   (nice_to_have before requirements, so "Preferred Qualifications" doesn't
   fall into requirements), applies the benefits cutoff.
3. Dictionary match — `CompileSkills`/`MatchSkills` against `data/skills.yaml`
   (~90 skills seeded across the §6 taxonomy, not the 250-400 month-3
   target); `patterns` escape hatch for tokens `\b` handles badly (C++, C#,
   .NET) or where case disambiguates real ambiguity (bare "go" vs. the Go
   language).

Stages 4 (Bedrock fallback) and 5 (review queue) are not built — out of
scope per the operator's explicit session instructions.

**Ingest CLI** (`cmd/ingest`): `ingest` runs the full pipeline per posting
and writes `PostingSkill` edges; `report` computes the ranked table fresh
from a full scan every run rather than via §4's write-time
`TransactWriteItems` counters — a deliberate simplification (that mechanism
exists for at-least-once queue consumers double-processing a posting, which
doesn't exist yet with no SQS and a single process) flagged in
`NEXT_STEPS.md` so it doesn't quietly become permanent once `cmd/worker`
becomes a real queue consumer.

**Validation gate** (`internal/extract/precision_test.go`, §6 "Validation"):
70 hand-labeled real postings (exceeds the 50 target) in
`testdata/labeled/*.json`, each a hermetic fixture (embedded raw HTML/Lever
content, no network or DB dependency). Result: **precision 1.000, recall
0.942**, comfortably clearing the 90% precision gate.

## What broke

Four real bugs, each found by testing against live data rather than trusting
synthetic fixtures or a clean `go build`:

1. **Benefits-cutoff logic** force-classified sections *after* the first
   benefits section to boilerplate, but left the benefits section itself
   still classified `benefits`. Caught immediately by a unit test encoding
   the segmentation design as reviewed at checkpoint 2 — the one bug in this
   list found by a test rather than by live data.
2. **Greenhouse's `content` field comes back HTML-entity-double-escaped**
   (literal `&lt;p&gt;` instead of `<p>`), so the parser saw one giant text
   node with zero real tags and silently returned nothing. Every synthetic
   fixture used plain HTML and passed regardless; only running against a
   real Riot Games posting surfaced it. Fixed with `html.UnescapeString`
   before parsing.
3. **A 34-minute ingest hang**, reported by the operator directly. Root
   cause was not the connectors — `docker logs` showed DynamoDB Local's
   SQLite backend crash-looping on a broken Windows Docker Desktop volume
   mount (`unable to open database file`). The port stayed open (a plain
   `curl` got a fast 400), so it looked alive, but real operations never
   got a response, and with zero timeouts anywhere in the ingest path that
   meant an unbounded hang instead of a fast, diagnosable error. Fixed by
   running DynamoDB Local `-inMemory` instead of a volume-backed `-dbPath`
   (no persistence needed for local dev; `make ingest` repopulates it
   anyway), plus hardening the whole connector path with real timeouts so
   this class of failure surfaces in seconds next time regardless of cause.
4. **Two major seed companies used non-standard section headings that
   silently zeroed out their extraction entirely** — not misclassification,
   total absence of any requirements/nice_to_have section:
   - Epic Games used "What we're looking for" as its requirements heading,
     matching no cue in §6's original list. Found because the first full
     `make ingest` run logged `skillEdges=0` for epic-games specifically
     while every other company was non-zero.
   - Roblox used `<p><strong>You Will</strong></p>` — a bolded paragraph,
     not a semantic `<h1>-<h6>` tag — for its headings, so Stage 1 never
     even saw a section boundary. Found while pulling raw HTML for the
     hand-labeling exercise, again by noticing roughly half of Roblox's
     postings had zero detected sections. Fixed by teaching Stage 1 to
     recognize a `<p>` whose entire content is a single `<strong>`/`<b>`
     child as a pseudo-heading — re-ingesting raised total skill edges from
     169 to 210 (Roblox 15→49, Discord 5→12, both affected by the same
     pattern).

A fifth gap was found and **deliberately not fixed**: a real Kabam (Lever)
posting uses free-form headings ("In this role, you can expect to:") that
match nothing, and — unrelated but co-located — some Kabam postings are in
French, which no amount of English cue-list tuning addresses. Decided to
accept this rather than chase arbitrary phrasing or add i18n support for a
5-company Phase 1 slice; documented in `classifyHeading`'s KNOWN GAP comment
and directly reflected in the validation set (one Kabam posting was hand-
labeled from its true raw content specifically to make this gap visible in
the recall number rather than defining it away).

## What would be done differently

- **Every one of the four fixed bugs was found by running against real data
  and reading the actual per-company/per-skill numbers, not by "it builds
  and the report prints something."** The Epic and Roblox bugs in
  particular would have shipped silently — the DoD's own `make report`
  output looked perfectly plausible with 0% of two seed companies' data
  actually contributing. Watching `skillEdges` per company in the ingest
  log, and later cross-checking hand-labeled ground truth against real
  posting text rather than trusting the extractor's own output, should be
  the default verification step for this kind of pipeline, not an
  afterthought.
- **The hand-labeling exercise for the validation gate is what surfaced the
  Roblox bug**, not a dedicated debugging session — doing §6's validation
  work earlier, in parallel with initial extraction development rather than
  as a final step, would likely have caught both heading bugs sooner.
- **Ground truth must come from the source text, not from the tool being
  validated.** It would have been faster to just copy the auto-matcher's
  own output as "ground truth" for the two Kabam postings that had zero
  matches — and it would have produced a meaningless, tautological 100%
  recall that actively hid the accepted gap instead of measuring it
  honestly.
