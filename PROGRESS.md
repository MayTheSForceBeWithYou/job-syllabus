# Progress

**Last updated:** 2026-08-13
**Commit:** `9c0dd83` ("Implement extraction Stage 1 (normalize) + Stage 2 (segment)")
**Branch:** `main`, matches `origin/main`

This supersedes the earlier Cursor-generated audit in this file, which was
written when the repo had only `docs/design.md`, `README.md`, and empty
directory stubs — no Go module existed yet. That audit also treated an
untracked `job-syllabus-design.md` as a competing source of truth; that's
resolved (see `docs/design-full-reference.md`'s header) and `docs/design.md`
is the only source of truth.

Scope for this session, per the operator's explicit instruction: Phase 0
(scaffold) and Phase 1 (local vertical slice) combined, targeting the DoD in
`docs/design.md` §17 — `make ingest && make report` prints a ranked
skill-frequency table from real postings across ≥5 companies.

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
- Explicit HTTP client timeout on the AWS SDK client (10s) — see the hang
  postmortem below.

**Connectors** (`internal/connectors`)
- `Connector` interface, `GreenhouseConnector`, `LeverConnector` against
  their real public JSON APIs. Registry keyed by ATS name.
- Hardened HTTP client: explicit dial/TLS-handshake/response-header
  sub-timeouts on top of a 15s overall `Client.Timeout`.
- `FetchWithRetry`: up to 2 retries with exponential backoff, `slog`
  WARN/ERROR logging with elapsed time.
- `internal/config`: loads and validates `data/companies.yaml` (unique
  slug, known tier, registered ATS, non-empty roleFilters) at load time.
- `internal/dedupe`: URL canonicalization + sha256-derived PostingID/
  ContentHash per §5.

**Ingest CLI** (`cmd/ingest`)
- `ingest` subcommand: loads `data/companies.yaml`, fetches via connector,
  client-side role-filters, canonicalizes + dedupes, upserts to DynamoDB
  Local. `slog` structured logging throughout (company start/complete,
  fetch timing, retries, skips). Overall run deadline (`INGEST_TIMEOUT`,
  default 15m) and per-company deadline (`COMPANY_TIMEOUT`, default 20s).
- `report` subcommand: **interim only** — prints posting counts per
  company. The real DoD deliverable (ranked skill table) needs Stage 3
  (dictionary matching), not built yet.
- **Verified against real data:** `make ingest` against all 5 seed
  companies completes in ~8s, 616 postings fetched, 71 role-matched and
  stored (epic-games 9, riot-games 26, discord 7, roblox 24, kabam 5).

**Extraction Stage 1 + 2** (`internal/extract`)
- Stage 1 (Normalize): `htmlToLines` via goquery — strips
  nav/footer/script/style, flattens to `"## heading"` / `"- item"` lines.
- Stage 2 (Segment): splits on heading markers, classifies each heading
  against ordered cue lists (nice_to_have checked before requirements),
  applies the benefits cutoff (first benefits section + everything after
  it → boilerplate).
- Lever's structured `lists` are used directly, bypassing HTML, per §5/§6.
- Unit tests cover classification and both the HTML and Lever-lists paths.
- **Verified against live postings**, which surfaced and fixed two real
  bugs (see postmortems below) plus one open question (see Next Steps).

## Not built yet

- Stage 3 (dictionary matching), `data/skills.yaml`, Stage 4 (Bedrock
  fallback), Stage 5 (review queue) — none implemented. This is what
  turns the interim posting-count report into the actual ranked
  skill-frequency table.
- `testdata/labeled/*.json` hand-labeled validation set + the 90%-precision
  test gate from §6 — not started.
- `docs/phase-0.md` / `docs/phase-1.md` writeups (§0.6 requires these at
  the end of every phase — pending until Stage 3 lands and the DoD passes).
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

**Two bugs only found by testing against live postings, not synthetic
fixtures:**
1. The benefits-cutoff logic force-classified sections *after* the first
   benefits section to boilerplate, but left the benefits section itself
   classified as `benefits` — a unit test encoding the reviewed design
   caught this immediately.
2. Greenhouse's `content` field comes back HTML-entity-double-escaped
   (literal `&lt;p&gt;` instead of `<p>`), so the HTML parser saw one giant
   text node with zero real tags and silently returned nothing. This only
   surfaced by running against a real Riot Games posting — every synthetic
   test fixture used plain HTML and passed regardless. Fixed with
   `html.UnescapeString` before parsing.

Both are a concrete argument for the "test against real postings, not just
synthetic fixtures" approach this session has been using at each checkpoint.
