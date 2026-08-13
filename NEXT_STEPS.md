# Next steps

**Single next phase: Phase 0 — Scaffold**  
Do not start Phase 1 (connectors, extraction, DynamoDB Local, `make ingest && make report`) until this phase’s Definition of Done passes.

---

## Definition of Done

`docs/design.md` does not contain §13 (the phased plan its own §0 names). The Phase 0 DoD is therefore missing from the source of truth. The only verbatim phase DoD is in untracked `job-syllabus-design.md` §13. **Copied verbatim from that file, not from `docs/design.md`:**

> **DoD:** `make test` and `make lint` pass on an empty test suite.

Closest Phase 0 *scope* text that **is** in `docs/design.md` (§17), copied verbatim:

> Start with the repo scaffold (cmd/{api,ingest,scraper,worker,rollup}, internal/{api,auth,connectors,extract,model,store,queue,export,safefetch,config}, data/, testdata/labeled/, docs/, Dockerfile, docker-compose.yml, Makefile)

Also from `docs/design.md` §0.6 (applies to every phase, including this one):

> **Every phase ends with a `docs/phase-N.md` writeup** — what was built, what broke, what you'd do differently. These become the portfolio blog posts, and they are not optional.

`docs/design.md` §17’s combined DoD (`make ingest && make report` … 5 companies) is **Phase 1**. Do not use it as the Phase 0 exit criterion.

---

## Open questions

`docs/design.md` has **no §16**. The open-question list lives only in `job-syllabus-design.md` §16.

**None of those five questions block Phase 0** (role-family classification, salary extraction, historical trends, guest read access, v2 ideas). Do not invent answers while scaffolding.

**Blocks this phase (documentation, not §16):** `docs/design.md` is truncated. Phase 0 can still be executed from §17’s scaffold list + the DoD above. **Do not restore or merge `job-syllabus-design.md` into `docs/design.md` as a silent side effect of Phase 0 unless the operator explicitly asks** — the SoT rule is that `docs/design.md` wins, and filling §10–§16 is a spec decision, not a code task. Flag it: later phases have **no DoD in the SoT file**. Fix that before Phase 1+ is treated as specified.

---

## Shaky prerequisites

There is no earlier DONE phase. Two things will make Phase 0’s own DoD lie if ignored:

1. **Go 1.23+ and `make` are not installed on this machine** (verified 2026-08-13). Docker is. `make test` / `make lint` cannot pass until the toolchain exists. The SoT specifies `make`, not a PowerShell substitute.
2. **`README.md` already claims Phase 0/1 is in progress.** That is false. Do not extend README’s connector/extraction story during Phase 0. A one-line “Phase 0 scaffold” status is enough when the writeup lands; do not advertise DynamoDB Local until it exists.

Empty `cmd/` and `internal/` directories are not a foundation — they are untracked name stubs. Re-creating packages on top of them is fine; do not assume they imply a module.

---

## Ordered task list (close the gap to DoD)

1. **Install toolchain:** Go 1.23+ on PATH; `make` (e.g. via Git for Windows / chocolatey `make`); `golangci-lint`. Confirm `go version` reports 1.23 or newer.
2. **`go mod init`** at repo root for a single module (five binaries, shared `internal/` — `docs/design.md` §3). Do not add dependencies yet. Approved list is supposed to be §14; that section is missing from the SoT. Phase 0 empty suite should not need `aws-sdk-go-v2` / `chi` / `goquery`.
3. **Makefile** with at least `test` → `go test ./...` and `lint` → `golangci-lint run`. No `ingest` / `report` targets required to *pass* Phase 0; adding empty stubs that fail is worse than omitting them until Phase 1.
4. **golangci-lint config** at repo root so `make lint` is deterministic.
5. **`.editorconfig`.**
6. **`docker-compose.yml`** with DynamoDB Local and LocalStack, as named in `job-syllabus-design.md` §13 / `docs/design.md` §17. Compose file on disk is Phase 0; **do not** implement store code or run ingest against it yet.
7. **`Dockerfile`** multi-stage, one build-arg selecting the binary (`docs/design.md` §17; detail in `job-syllabus-design.md` §14). Can be a compile-only stub; do not push images (Jenkins is Phase 2; §0.4).
8. **Minimal Go packages so `go test ./...` has a module to test:** stub `cmd/{api,ingest,scraper,worker,rollup}/main.go` and empty `internal/*` packages if needed. **No connector implementations, no extraction pipeline, no DynamoDB access, no HTTP API, no Terraform, no Jenkins, no Cognito, no Expo.** `docs/design.md` §17: show the `Connector` interface and `companies.yaml` schema *before* writing connector code — that review is the start of Phase 1, not Phase 0.
9. **Keep `data/` and `testdata/labeled/` in git** (`.gitkeep` or equivalent). Do not add `companies.yaml` / `skills.yaml` / labeled JSON until Phase 1 — those are the vertical slice, not the empty suite.
10. **Run `make test` and `make lint` and keep both green** on that empty suite. That is the DoD.
11. **`docs/phase-0.md` writeup** (what was built, what broke, what you’d do differently). Required by §0.6.
12. **Correct `README.md`** so it does not claim Phase 1 work exists. Point at `docs/design.md` and the new phase-0 writeup.

Stop. Do not research ATS tokens, do not hand-label postings, do not write `internal/connectors` or `internal/extract` beyond empty packages.
