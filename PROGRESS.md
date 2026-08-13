# Progress audit

**Last verified:** 2026-08-13  
**Commit:** `1c55fd3fadebaffe71d3f30eba4d274f5e80d0da` (`Add design doc, README, and gitignore`, 2026-08-13 11:07:03 -0700)  
**Branch:** `main` (matches `origin/main`)  
**Working tree:** untracked `job-syllabus-design.md` plus empty untracked directory stubs (`cmd/`, `internal/`, `data/`, `testdata/`). No application code.

This is a command-verified audit against `docs/design.md`. No application code was added or changed.

---

## Source-of-truth conflict (read this first)

`docs/design.md` is the mandated source of truth. It is **incomplete**.

- `docs/design.md` §0 says: build in the phase order in **§13**; approved deps in **§14**. Those sections are not in the file.
- After a truncated §9 layout tree, `docs/design.md` replaces the rest of infrastructure plus **§10–§16** with a one-paragraph summary, then jumps to §17.
- Untracked `job-syllabus-design.md` (repo root, not in the §14 layout) contains the missing material. Its **§13** is the phased build plan (Phases 0–8 with Definitions of Done). Its **§16** is Open Questions.
- `job-syllabus-design.md` §0 **disagrees with its own headings**: it says phases are in §12 and deps in §13, but the content is at §13 / §14. Several other cross-refs in that file are off by one vs `docs/design.md` (e.g. SSRF “see §10” vs `docs/design.md` “see §11”). **`docs/design.md` wins** where they conflict; the gap is that `docs/design.md` has the correct pointers and no targets.

**Phase list used below** comes from `job-syllabus-design.md` §13, because that is the only copy of the plan `docs/design.md` §0 names. This is a hole in the SoT, not a silent switch of spec.

`README.md` conflicts with both the SoT and the repo: it says “Currently in progress: **Phase 0/1**” (connectors, extraction, DynamoDB Local). There is no Go module, no Makefile, no connectors, no extraction, no compose stack. README is aspirational, not status.

---

## Current phase

**Frontier: Phase 0 — Scaffold — IN PROGRESS** (DoD does not pass.)

Evaluation stopped here. Later phases were not executed.

| Phase | Name | Status | How verified |
|---|---|---|---|
| 0 | Scaffold | **IN PROGRESS** | `make test` / `make lint` cannot run (see errors below). Empty dir stubs exist; no Makefile, no Go module, no lint config, no compose, no Dockerfile, no `.editorconfig`. |
| 1 | Local vertical slice | NOT STARTED | Not evaluated. Blocked on Phase 0. |
| 2 | Terraform baseline + Jenkins | NOT STARTED | Not evaluated. |
| 3 | Deploy API | NOT STARTED | Not evaluated. |
| 4 | Ingestion at scale | NOT STARTED | Not evaluated. |
| 5 | Bedrock + review queue | NOT STARTED | Not evaluated. |
| 6 | Auth + client | NOT STARTED | Not evaluated. |
| 7 | Scraper + share extension | NOT STARTED | Not evaluated. |
| 8 | Export, trends, polish, writeup | NOT STARTED | Not evaluated. |

Phase 0 DoD (only copy; `job-syllabus-design.md` §13 — **absent from `docs/design.md`**):

> **DoD:** `make test` and `make lint` pass on an empty test suite.

`docs/design.md` §17 does not give a Phase-0-only DoD. It folds Phase 0+1 into one line:

> Definition of done: `make ingest && make report` prints a ranked table from real postings for at least 5 companies.

That combined line is Phase 1, not Phase 0. Using it as Phase 0 DoD would skip the scaffold gate that §0 requires.

---

## Phase 0 — built vs broken vs missing

### Built

- Tracked: `.gitignore`, `README.md`, `docs/design.md`.
- Untracked draft: `job-syllabus-design.md` (full design text; not part of the repo layout).
- Empty local directories matching `docs/design.md` §17 / `job-syllabus-design.md` §14 names. Git does not track them. No files inside.

```
cmd/{api,ingest,scraper,worker,rollup}/
internal/{api,auth,config,connectors,export,extract,model,queue,safefetch,store}/
data/
testdata/labeled/
docs/
```

These are name-only stubs, not packages.

### Built but broken / failing

Nothing compiles or is testable. The DoD commands fail before they can exercise an empty suite:

1. **`make` is not on PATH** (Windows).
2. **No `Makefile`** in the repo, so the targets would fail even with `make` installed.
3. **`go` is not on PATH.** `docs/design.md` §2 locks Go 1.23+.

### Not started (Phase 0 scope from `docs/design.md` §17 + `job-syllabus-design.md` §13)

| Artifact | Status |
|---|---|
| `go.mod` (Go 1.23+) | missing |
| `Makefile` (`test`, `lint`; later `ingest` / `report`) | missing |
| `golangci-lint` config | missing |
| `docker-compose.yml` (DynamoDB Local + LocalStack) | missing |
| `.editorconfig` | missing |
| `Dockerfile` (multi-stage, binary selected by arg) | missing |
| Any `main.go` under `cmd/*` | missing |
| Any `.go` under `internal/*` | missing |
| `data/companies.yaml`, `data/skills.yaml` | missing (`data/` is empty) |
| `testdata/labeled/*.json` | missing (dir empty) |
| `docs/phase-0.md` writeup (`docs/design.md` §0.6 — not optional) | missing |
| `infra/`, `ci/`, `mobile/` | correctly absent (Phase 2+ / 6+) |

---

## Extraction-precision gate (§6)

**Not run. No number.**

`docs/design.md` §6: hand-label 50 postings in `testdata/labeled/*.json`; `go test ./internal/extract` fails the build below **0.90 precision**; do this in **Phase 1**, not Phase 8.

- `internal/extract/` is an empty directory.
- `testdata/labeled/` is empty.
- `go` is not installed, so the gate cannot be executed.

**Later phases are not safe to build on this.** There is no extraction, no labeled set, and no precision result. Treat 0.90 as unmet, not as “unknown, proceed.”

`job-syllabus-design.md` Phase 1 DoD includes this gate. `docs/design.md` §17’s one-line DoD does not mention it; **§6 still does**, and §6 is in the SoT. Do not drop the gate.

---

## Deviations from `docs/design.md`

No application code exists, so there is no DynamoDB key-schema drift, connector-interface drift, or skipped runtime validation in code. Documentation / process drift:

1. **`docs/design.md` is not the document it claims to be.** §0, §5, §6, §15-style pointers target §10–§16, which were summarized out. Implementers cannot follow “phase order in §13” from the SoT file alone.
2. **`README.md` reports Phase 0/1 in progress** (Greenhouse/Lever, dictionary extraction, DynamoDB Local). That work is not in the tree. Do not treat README as status.
3. **`job-syllabus-design.md` at repo root** is outside the layout in §14 / §17. Untracked. Its §0 section numbers conflict with `docs/design.md` and with its own headings.
4. **Empty `cmd/` / `internal/` trees** match the layout but are not a Go module. Creating dirs without `go.mod` / stubs does not satisfy Phase 0.
5. **No `docs/phase-N.md` writeups.** §0.6 says they are required at the end of every phase.
6. **Toolchain vs §2.** This machine has Docker; it does not have Go 1.23+ or `make`. Phase 0 DoD cannot pass here until those exist (or the Makefile documents a Windows-native equivalent — the SoT specifies `make`, not `nmake`/PowerShell).

No Terraform, Jenkins, auth, or Expo code — that matches `docs/design.md` §17 (“Do not write any Terraform, Jenkins configuration, auth, or Expo code yet”).

---

## Failing tests / commands (actual output)

### `make test`

```
make: command not found
```

`where.exe make` → `INFO: Could not find files for the given pattern(s).`  
There is also no `Makefile`.

### `make lint`

```
make: command not found
```

Same as above. No `.golangci.yml` / `.golangci.yaml`.

### `make ingest` / `make report`

Not required for Phase 0. Invoked to confirm Phase 1 is not secretly done:

```
make: command not found
```

No ingest/report targets, no binaries, no company registry.

### `go test ./internal/extract`

```
go: command not found
```

`where.exe go` → not found.

### `terraform plan`

Not a Phase 0 command. `terraform` is not on PATH; `infra/terraform/` does not exist. Not run further.

### Docker

`docker` is present (`C:\Program Files\Docker\Docker\resources\bin\docker.exe`). No `docker-compose.yml` to use it with.
