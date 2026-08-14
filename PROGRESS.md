# Progress

**Last updated:** 2026-08-13
**Commit:** `5becb90` ("Add hand-labeled validation set + precision/recall gate (§6)")
**Branch:** `main`, matches `origin/main`

This supersedes the earlier Cursor-generated audit in this file, which was
written when the repo had only `docs/design.md`, `README.md`, and empty
directory stubs — no Go module existed yet. That audit also treated an
untracked `job-syllabus-design.md` as a competing source of truth; that's
resolved (see `docs/design-full-reference.md`'s header) and `docs/design.md`
is the only source of truth.

Scope for this session, per the operator's explicit instruction: Phase 0
(scaffold) and Phase 1 (local vertical slice) combined, targeting the DoD in
`docs/design.md` §17. **Phase 0 and Phase 1 are both complete** — see
[docs/phase-0.md](docs/phase-0.md) and [docs/phase-1.md](docs/phase-1.md)
for the full writeups (§0.6 requires these; they're the more detailed
companion to this file).

## DoD: met

> `make ingest && make report` prints a ranked table from real postings for
> at least 5 companies.

```
$ make ingest && make report
...
=== Skill frequency across 70 postings (5 companies) ===
SKILL                        CATEGORY     COUNT % OF POSTS  REQ'D NICE-TO-HAVE
Amazon Web Services (AWS)    cloud           16      22.9%     12            4
Kubernetes                   containers      16      22.9%     12            4
Google Cloud Platform (GCP)  cloud           14      20.0%     11            3
C++                          languages       13      18.6%     12            1
Go                           languages       12      17.1%     11            1
Python                       languages       12      17.1%     11            1
Microsoft Azure              cloud           10      14.3%      9            1
...
41 distinct skills matched across 70 postings
```

Real postings from Epic Games, Riot Games, Discord, Roblox (Greenhouse) and
Kabam (Lever) — no mocking, no fixtures. Full pipeline: fetch → role-filter →
dedupe → normalize → segment → dictionary-match → store → rank. Numbers
above are a live snapshot and will drift run to run as real postings
open/close.

**Validation gate: passing.** `go test ./internal/extract -run
TestExtractionPrecisionRecall -v`: 70 hand-labeled postings, **precision
1.000, recall 0.942** — well clear of §6's 90% precision gate.

---

## What's built and verified working

**Toolchain & scaffold**
- Go 1.23+ module (`go.mod`), `cmd/{api,ingest,scraper,worker,rollup}`
  (api/scraper/worker/rollup are panic-stubs — not implemented, correctly
  out of scope this phase), `internal/{model,store,connectors,config,
  dedupe,extract}`.
- `Makefile` (build/test/lint/fmt/up/down/ingest/report), `docker-compose.yml`
  (DynamoDB Local), `Dockerfile` (multi-stage, `--build-arg BINARY` selects
  which `cmd/` binary — not wired to any deploy path), `.editorconfig`.
- `lint` runs `golangci-lint run ./...` (config: `.golangci.yml`, standard
  linter set + `unconvert`/`unparam`). Caught 2 real findings on first run
  (unchecked `resp.Body.Close()` errors in both connectors) — fixed.

**Data model & store** (`internal/model`, `internal/store`)
- Single-table item shapes from §4: `Posting`, `PostingSkill`, `Skill`.
- DynamoDB Local client with `EnsureTable` (idempotent create + 2 GSIs).
- `UpsertPosting` returns the final stored posting (not just created/err)
  so callers can layer extraction results on top of the correct
  FirstSeenAt/etc. rather than their pre-upsert draft.
- `PutSkillEdge` / `ListAllSkillEdges` for Posting→Skill edges — a plain
  idempotent PutItem + Scan, not the write-time `TransactWriteItems`
  counter mechanism §4 describes. Deliberate Phase 1 simplification: that
  mechanism exists to survive an at-least-once queue consumer
  double-processing a posting, which doesn't exist yet (no SQS, single
  process, no concurrent writers). `report` recomputes counts from source
  of truth every run instead — simpler, and nothing to drift.

**Connectors** (`internal/connectors`)
- `Connector` interface, `GreenhouseConnector`, `LeverConnector` against
  their real public JSON APIs. Registry keyed by ATS name.
- Hardened HTTP client: explicit dial/TLS-handshake/response-header
  sub-timeouts on top of a 15s overall `Client.Timeout`.
- `FetchWithRetry`: up to 2 retries with exponential backoff, `slog`
  WARN/ERROR logging with elapsed time.
- `internal/config`: loads/validates `data/companies.yaml` and
  `data/skills.yaml` at load time — fail fast, not per-item at use time.
- `internal/dedupe`: URL canonicalization + sha256-derived PostingID/
  ContentHash per §5.

**Ingest CLI** (`cmd/ingest`)
- `ingest`: loads companies + skills, fetches via connector, client-side
  role-filters, canonicalizes + dedupes, upserts to DynamoDB Local, then
  runs Stages 1-3 on the posting immediately and writes the resulting
  skill edges. `slog` structured logging throughout. Overall run deadline
  (`INGEST_TIMEOUT`, default 15m) and per-company deadline
  (`COMPANY_TIMEOUT`, default 20s).
- `report`: prints the real ranked skill-frequency table — count, % of
  postings, required vs. nice-to-have split, sorted by count. Column
  widths size to the widest actual value each run.

**Extraction pipeline** (`internal/extract`) — Stages 1-3 of 5
- Stage 1 (Normalize): `htmlToLines` via goquery — strips
  nav/footer/script/style, flattens to `"## heading"` / `"- item"` lines.
  Unescapes HTML entities first, and recognizes a `<p>` whose entire
  content is one `<strong>`/`<b>` child as a pseudo-heading (see
  postmortems below for why both exist).
- Stage 2 (Segment): splits on heading markers, classifies each heading
  against ordered cue lists (nice_to_have checked before requirements),
  applies the benefits cutoff. Lever's structured `lists` are used
  directly, bypassing HTML, per §5/§6.
- Stage 3 (Dictionary match): `CompileSkills` builds a case-insensitive
  `\bAlias\b` regex per alias, or uses author-supplied `patterns` verbatim
  for tokens `\b` handles badly (C++, C#, .NET) or where case matters (Go
  vs. "go"). `MatchSkills` scans requirements/nice_to_have sections only;
  a skill matched in both gets Required=true, not overwritten.
- Stage 4 (Bedrock fallback) and Stage 5 (review queue): not built.
- `data/skills.yaml`: ~90 skills seeded across the §6 taxonomy (not the
  250-400 month-3 target). `meta` category (years of experience, degree,
  shipped-title) deliberately not seeded — needs separate structured-fact
  extraction, not dictionary matching.
- Unit tests cover classification, both HTML/Lever-lists segmentation
  paths, dictionary matching (case-sensitivity, required-wins, evidence
  truncation, requirements-sections-only scope), and the pseudo-heading
  normalize path.

**Validation gate** (`internal/extract/precision_test.go`, §6)
- 70 hand-labeled real postings in `testdata/labeled/*.json` (exceeds the
  50-posting target), one hermetic fixture per posting (embedded raw
  HTML/Lever content — no network or DynamoDB dependency).
- Ground truth built by reading actual requirement bullets against
  `data/skills.yaml`, not by trusting the extractor's own output — this is
  what caught the Roblox pseudo-heading bug (see postmortems).
- **Result: precision 1.000 (0 false positives / 210 true positives),
  recall 0.942 (13 false negatives, all from one Kabam posting hand-labeled
  specifically to make the accepted heading-gap visible in the number
  rather than defining it away).**

## Not built yet

- Stage 4 (Bedrock fallback), Stage 5 (review queue).
- Everything explicitly out of scope this session: Terraform, Jenkins, auth,
  Expo/mobile client, role-family classification, `meta`-category
  structured-fact extraction.

---

## Postmortems worth keeping

**The 34-minute ingest hang.** Root cause was *not* the ATS connectors —
it was DynamoDB Local's SQLite backend failing to open its database file on
a Docker Desktop (Windows) volume mount, crash-looping every 3s. The port
stayed open (a plain `curl` got a fast HTTP 400), so it looked alive, but
real DynamoDB operations never got a response — and with zero timeouts
anywhere in the code, that meant an unbounded hang instead of a fast, loud
error. Fixed by running DynamoDB Local with `-inMemory` instead of a
volume-backed `-dbPath`. Separately hardened the whole path (explicit
sub-timeouts, per-company deadlines, bounded retry, structured logging) so
this class of failure surfaces in seconds next time, whatever causes it.

**Four bugs only found by testing against live data, not synthetic
fixtures or a clean build:**
1. The benefits-cutoff logic force-classified sections *after* the first
   benefits section to boilerplate, but left the benefits section itself
   classified `benefits` — a unit test encoding the reviewed design caught
   this immediately.
2. Greenhouse's `content` field comes back HTML-entity-double-escaped
   (literal `&lt;p&gt;` instead of `<p>`), so the HTML parser saw one giant
   text node with zero real tags and silently returned nothing. Only
   surfaced by running against a real Riot Games posting. Fixed with
   `html.UnescapeString` before parsing.
3. Epic Games used "What we're looking for" as its requirements heading,
   matching no §6 cue. Every Epic posting contributed zero skills until
   the first full `make ingest` run showed `skillEdges=0` for that company
   specifically while every other company was non-zero. Fixed directly —
   a single well-defined phrase from a major seed company, not open-ended
   arbitrary phrasing.
4. Roblox used `<p><strong>You Will</strong></p>` — a bolded paragraph,
   not a semantic heading tag — so Stage 1 never saw a section boundary at
   all. Found while pulling raw content for hand-labeling, again by
   noticing roughly half of Roblox's postings had zero detected sections.
   Fixed by teaching Stage 1 to recognize a `<p>` whose entire content is
   one bold child as a pseudo-heading. Re-ingesting raised total skill
   edges from 169 to 210 (Roblox 15→49, Discord 5→12 — both use the
   pattern).

All four are the same lesson from different angles: check the actual
per-company/per-skill numbers, not just "did it run." Bugs #3 and #4 in
particular would have shipped a DoD-passing report with two of five seed
companies' data silently near-empty.

**Accepted, not fixed:** Kabam (Lever) uses free-form headings ("In this
role, you can expect to:") that match nothing, and some Kabam postings are
in French. Documented in `classifyHeading`'s KNOWN GAP comment and reflected
honestly in the validation set — see `NEXT_STEPS.md`.
