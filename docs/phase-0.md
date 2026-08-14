# Phase 0 — Scaffold

**Status: done.** DoD per the reference doc (§13): `make test` and `make lint` pass.
In practice this phase and Phase 1 were built together in one session per explicit
operator instruction, so this writeup and [phase-1.md](phase-1.md) cover the same
commit range and should be read as a pair.

## What was built

- Go 1.23+ module (`go.mod`) with the layout from `docs/design.md` §17:
  `cmd/{api,ingest,scraper,worker,rollup}`, `internal/{model,store,connectors,
  config,dedupe,extract}`, `data/`, `testdata/labeled/`, `docs/`.
- `Makefile` (`build`, `test`, `lint`, `fmt`, `up`, `down`, `ingest`, `report`,
  `clean`), `docker-compose.yml` (DynamoDB Local), `Dockerfile` (multi-stage,
  `--build-arg BINARY` selects which `cmd/` binary — not wired to any deploy
  path yet), `.editorconfig`, `.golangci.yml`.
- `cmd/{api,scraper,worker,rollup}` are panic-stub `main.go` files — real Go
  packages, not empty directories, but intentionally unimplemented until their
  phases. Only `cmd/ingest` does real work in Phase 0/1.

## What broke

**The design-doc source-of-truth conflict.** The operator pasted `docs/design.md`
into chat with §9-16 deliberately compressed to one paragraph, and explicitly
scoped this session to Phase 0+1 combined. A separate uncompressed version of the
doc (`job-syllabus-design.md`) was sitting in the working directory — the
operator's own original file. Partway through the session, a Cursor-generated
audit (written into `PROGRESS.md`/`NEXT_STEPS.md` before any Go code existed)
found that file and treated it as more authoritative than `docs/design.md`,
deriving a stricter "Phase 0 must be an empty test suite" gate from its phase
list. That's backwards: `docs/design.md` is what the operator actually
instructed against for this session. Resolved by moving the file to
`docs/design-full-reference.md` with a header stating `docs/design.md` wins on
any conflict, and continuing under the operator's original combined-phase scope.
Lesson: when two documents disagree, check which one a human actually pointed
you at in the current conversation before trusting a third tool's inference
about which is authoritative.

**Docker Desktop wasn't running, twice**, both times requiring a ~3.5 minute
cold start (launch, then poll `docker info` until it responds) before any
`docker compose` command would succeed. Not a bug, just a real environmental
gate worth naming since it blocked work both times it came up.

**No `golangci-lint` initially.** `lint` ran `go vet` + `gofmt -l` as a
placeholder until it got installed mid-session, at which point `.golangci.yml`
(v2 config schema — the installed version is v2.12.2, whose config format
differs from v1) was added and the Makefile target switched over. First real
run caught 2 genuine findings (unchecked `resp.Body.Close()` errors in both
ATS connectors) — see [phase-1.md](phase-1.md) for what that means for the
connectors themselves.

## What would be done differently

- **Install and verify the full toolchain (Go, `make`, Docker Desktop running,
  `golangci-lint`) before writing any code**, not partway through. Every one of
  the above interruptions was a toolchain gap discovered by trying to use it,
  not by checking first.
- **Resolve document conflicts explicitly and immediately**, the moment a
  second source of "truth" appears in the working directory, rather than
  letting a downstream tool's inference about authority stand unchallenged
  until it visibly causes drift.
- LocalStack is named alongside DynamoDB Local in the reference doc's Phase 0
  scaffold list but deliberately **not** set up here — nothing in Phase 0/1
  scope touches S3 (postings' raw HTML isn't persisted anywhere yet; see
  `Posting.RawS3Key` sitting unused in `internal/model`). Standing up
  LocalStack now would be inert infrastructure. Add it when `RawS3Key` is
  actually written to, not before.
