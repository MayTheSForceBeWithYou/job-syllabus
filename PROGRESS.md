# Progress

**Last updated:** 2026-08-13
**Commit:** `ea09ef6` ("Implement Stage 3 (dictionary matching); DoD met end to end")
**Branch:** `main`, matches `origin/main`

This supersedes the earlier Cursor-generated audit in this file, which was
written when the repo had only `docs/design.md`, `README.md`, and empty
directory stubs — no Go module existed yet. That audit also treated an
untracked `job-syllabus-design.md` as a competing source of truth; that's
resolved (see `docs/design-full-reference.md`'s header) and `docs/design.md`
is the only source of truth.

Scope for this session, per the operator's explicit instruction: Phase 0
(scaffold) and Phase 1 (local vertical slice) combined, targeting the DoD in
`docs/design.md` §17.

## DoD: met

> `make ingest && make report` prints a ranked table from real postings for
> at least 5 companies.

```
$ make ingest && make report
...
=== Skill frequency across 70 postings (5 companies) ===
SKILL                        CATEGORY        COUNT % OF POSTS  REQ'D NICE-TO-HAVE
Amazon Web Services (AWS)    cloud              12      17.1%      9            3
Kubernetes                   containers         11      15.7%      8            3
Google Cloud Platform (GCP)  cloud              10      14.3%      8            2
C++                          languages           9      12.9%      8            1
Go                           languages           8      11.4%      7            1
Python                       languages           8      11.4%      7            1
Terraform                    cloud               8      11.4%      3            5
...
41 distinct skills matched across 70 postings
```

Real postings from Epic Games, Riot Games, Discord, Roblox (Greenhouse) and
Kabam (Lever) — no mocking, no fixtures. Full pipeline: fetch → role-filter →
dedupe → normalize → segment → dictionary-match → store → rank.

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
- `internal/config`: loads/validates `data/companies.yaml` and (new)
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
  postings, required vs. nice-to-have split, sorted by count.
- **Verified against real data** (see DoD output above): 617 postings
  fetched, 70 role-matched and stored, 169 skill edges written, 41
  distinct skills ranked.

**Extraction pipeline** (`internal/extract`) — Stages 1-3 of 5
- Stage 1 (Normalize): `htmlToLines` via goquery — strips
  nav/footer/script/style, flattens to `"## heading"` / `"- item"` lines.
  Unescapes HTML entities first (see postmortem below).
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
  paths, and dictionary matching (case-sensitivity, required-wins,
  evidence truncation, requirements-sections-only scope).

## Not built yet

- Stage 4 (Bedrock fallback), Stage 5 (review queue).
- `testdata/labeled/*.json` hand-labeled validation set + the 90%-precision
  test gate from §6 — not started.
- `docs/phase-0.md` / `docs/phase-1.md` writeups (§0.6 requires these —
  next up now that the DoD passes).
- Everything explicitly out of scope this session: Terraform, Jenkins, auth,
  Expo/mobile client.

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

**Three bugs only found by testing against live postings, not synthetic
fixtures — each caught during the actual session, not after:**
1. The benefits-cutoff logic force-classified sections *after* the first
   benefits section to boilerplate, but left the benefits section itself
   classified as `benefits` — a unit test encoding the reviewed design
   caught this immediately.
2. Greenhouse's `content` field comes back HTML-entity-double-escaped
   (literal `&lt;p&gt;` instead of `<p>`), so the HTML parser saw one giant
   text node with zero real tags and silently returned nothing. Only
   surfaced by running against a real Riot Games posting. Fixed with
   `html.UnescapeString` before parsing.
3. Epic Games — one of the 5 seed companies — uses "What we're looking
   for" as its requirements heading, which matched no cue at all. Every
   single Epic posting contributed zero skills until the first full
   `make ingest` run showed `skillEdges=0` for that company specifically
   (all others were non-zero), which is what triggered the investigation.
   Unlike the already-accepted Kabam heading gap (see NEXT_STEPS.md), this
   was one well-defined phrase from a major seed company, not open-ended
   arbitrary phrasing, so it was fixed directly.

All three are a concrete argument for "test against real postings, check
the actual numbers, not just that the code runs without error" — bug #3 in
particular would have silently produced a DoD-passing report with a whole
seed company's data missing if the per-company log output hadn't been
checked line by line.
