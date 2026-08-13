# Job Syllabus — Design & Build Plan

**A system that ingests game-industry job postings, extracts skill requirements, and ranks them by frequency — turning the job market into a study syllabus.**

Planning document for implementation with Claude Code.

---

## 0. Read This First (Claude Code Working Agreement)

This document is the source of truth for architecture. When implementing:

1. **Build in the phase order in §12.** Each phase has a stated "definition of done." Do not start a phase until the previous one's DoD passes.
2. **Never invent AWS resource configuration.** If Terraform for a resource isn't specified here, ask before writing it.
3. **All infrastructure is Terraform.** No console click-ops, no `aws` CLI mutations except in explicitly-marked bootstrap scripts.
4. **All deploys go through Jenkins after Phase 2.** If you find yourself running `docker push` by hand, stop and fix the pipeline instead.
5. **Go code targets Go 1.23+, standard library first.** Approved external deps are listed in §13. Adding a dependency requires justification in the PR description.
6. **Every phase ends with a `docs/phase-N.md` writeup** — what was built, what broke, what you'd do differently. These become the portfolio blog posts, and they are not optional. The writeups are half the point of this project.

---

## 1. Problem Statement

The career roadmap called for building "a spreadsheet of repeated requirements" from ~20 studios' job postings, on the grounds that the postings are ground truth and everything else is prediction. Doing that by hand is tedious, produces a stale snapshot, and covers 20 studios instead of 200.

This project automates it. The output is a ranked, filterable, continuously-refreshed table answering: **across every build/release/live-ops engineering posting in the game industry right now, which specific skills appear most often?**

Secondarily — and this is roughly equal in value — the project is a portfolio artifact demonstrating Go, containers on AWS, Terraform, Jenkins-based CI/CD, and a cross-platform mobile client. It is deliberately built with the stack that game-adjacent platform companies actually run.

There is a pleasing recursion here: the app will tell you whether the effort spent on Jenkins, Terraform, and Perforce was correctly prioritized. Check its output against this plan at month 3 and be willing to be wrong.

### Goals

- **G1.** Ingest and store ≥500 postings across ≥40 companies, refreshed daily.
- **G2.** Extract normalized skill requirements with ≥90% precision on a hand-labeled 50-posting validation set.
- **G3.** Produce a ranked skill-frequency view, filterable by role family and company tier, exportable to CSV/XLSX.
- **G4.** Deliver it on iOS, Android, and web from one codebase.
- **G5.** Full stack reproducible from an empty AWS account via documented bootstrap + `terraform apply` + Jenkins seed job.
- **G6.** Run cost under $40/month steady-state.

### Non-Goals (v1)

- Not a job board. No apply flow, no alerts, no recommendation engine.
- Not an application tracker (candidate for v2).
- Not multi-tenant. Single operator (you) plus optional read-only guests.
- No resume gap-analysis / matching. Tempting, and a good v2, but it doubles scope.
- No republishing of posting text. See §10 on why this constraint is load-bearing.

### Success Criteria

The project succeeds if, at the end, you can say: *"Here is the ranked list of the 60 skills most frequently demanded in game build/release engineering, derived from N postings across M companies, and here is the running system that produces it."* Everything else is scaffolding around that sentence.

---

## 2. Stack Decisions (Locked)

| Layer | Choice | Notes |
|---|---|---|
| Backend language | **Go 1.23+** | Aligns with Epic/Riot/Roblox platform teams |
| Compute | **ECS Fargate** | Containers, not serverless — deliberate résumé choice |
| API edge | **API Gateway HTTP API** → VPC Link → internal ALB → Fargate | JWT authorizer at the edge |
| Data store | **DynamoDB** (single-table) | Plus S3 for raw snapshots |
| Auth | **Amazon Cognito** user pool | OIDC + PKCE from the client |
| Ingestion | **ATS public APIs** + **headless scraper** (chromedp) + **manual link submit** | Three-tier, in that priority order |
| Extraction | **Hybrid**: skill-dictionary pass → **Amazon Bedrock** (Claude) for unmatched spans | Dictionary compounds over time |
| IaC | **Terraform** | Doubles as Terraform Associate study material |
| Client | **Expo (dev client) + Expo Web** | One codebase → iOS, Android, web |
| CI/CD | **Jenkins on EC2**, configured via JCasC, ephemeral Fargate agents | The games-industry reality |

### Two honest tradeoff notes

**Jenkins is the expensive choice, and it's the right one.** GitHub Actions with OIDC would be roughly a tenth of the work and cost nothing. You picked Jenkins because studio postings ask for Jenkins, and the résumé line *"administered a Jenkins controller managed entirely as code (JCasC) with ephemeral Fargate build agents and zero static AWS credentials"* is worth more than the time it costs. Accept that Phase 2 will be the slowest phase. The mitigations are in §9.

**Fargate over Lambda costs you ~$15/month for the same workload.** Also deliberate. Container packaging, task definitions, service deployment, and rollback are the skills being demonstrated; Lambda would hide all of them.

---

## 3. Architecture

```
                    ┌──────────────────────────────────────────┐
   iOS / Android    │  Expo app (dev client)                   │
   Web browser      │  Expo Web → S3 + CloudFront              │
                    └───────────────┬──────────────────────────┘
                                    │ HTTPS + Cognito JWT
                          ┌─────────▼──────────┐
                          │ API Gateway        │
                          │ HTTP API           │
                          │ + JWT authorizer   │
                          └─────────┬──────────┘
                                    │ VPC Link
                          ┌─────────▼──────────┐
                          │ Internal ALB       │
                          └─────────┬──────────┘
                                    │
                          ┌─────────▼──────────┐        ┌──────────────┐
                          │ ECS Fargate        │───────►│  DynamoDB    │
                          │ service: api (Go)  │        │  single-table│
                          └─────────┬──────────┘        └──────▲───────┘
                                    │ enqueue                  │
                          ┌─────────▼──────────┐               │
                          │ SQS: ingest-queue  │               │
                          │     extract-queue  │               │
                          │     (+ DLQs)       │               │
                          └─────────┬──────────┘               │
                                    │                          │
        ┌───────────────────────────┼──────────────────────────┤
        │                           │                          │
┌───────▼────────┐        ┌─────────▼────────┐       ┌─────────▼────────┐
│ ECS task:      │        │ ECS service:     │       │ ECS task:        │
│ ingest         │        │ extract worker   │       │ scraper          │
│ (ATS connectors)│       │ dict → Bedrock   │       │ (chromedp)       │
└───────┬────────┘        └─────────┬────────┘       └─────────┬────────┘
        │                           │                          │
        │                  ┌────────▼─────────┐                │
        │                  │ Amazon Bedrock   │                │
        │                  │ (Claude)         │                │
        │                  └──────────────────┘                │
        └────────────────┬──────────────────────────────────────┘
                         │ raw snapshots
                  ┌──────▼────────┐
                  │ S3: raw/      │  (private, lifecycle → Glacier @ 90d)
                  │     exports/  │
                  └───────────────┘

  EventBridge Scheduler ──► RunTask(ingest)  daily 06:00 UTC
                       └──► RunTask(rollup)  daily 07:00 UTC

  Jenkins (EC2 t4g.small) ──► ECR push, ECS deploy, terraform apply, EAS build
```

### Component responsibilities

| Component | Responsibility |
|---|---|
| `cmd/api` | REST API, auth enforcement, reads DynamoDB, enqueues manual submissions, generates exports |
| `cmd/ingest` | Runs ATS connectors on a schedule, dedupes, writes raw to S3, enqueues extract jobs |
| `cmd/scraper` | Headless-browser fetch for sites without an API; consumes from ingest-queue |
| `cmd/worker` | Consumes extract-queue: normalize → segment → dictionary match → Bedrock fallback → write edges → increment counters |
| `cmd/rollup` | Nightly reconciliation of aggregate counters; recomputes from source of truth |

Everything is one Go module producing five binaries from a shared `internal/` tree. Do not split into microservice repos; the coupling is real and the ceremony isn't worth it.

---

## 4. Data Model (DynamoDB Single Table)

Table name: `jobsyllabus`. Billing mode: **on-demand** (PAY_PER_REQUEST). Point-in-time recovery: on.

**Attributes:** `PK`, `SK`, `GSI1PK`, `GSI1SK`, `GSI2PK`, `GSI2SK`, `entityType`, plus per-entity fields.

### Item shapes

| Entity | PK | SK | GSI1PK / GSI1SK | GSI2PK / GSI2SK |
|---|---|---|---|---|
| Company | `COMPANY#<slug>` | `META` | `TIER#<tier>` / `NAME#<name>` | — |
| Posting | `POSTING#<id>` | `META` | `COMPANY#<slug>` / `POSTED#<iso8601>` | `ROLE#<family>` / `POSTED#<iso8601>` |
| Posting→Skill edge | `POSTING#<id>` | `SKILL#<skillId>` | — | `SKILL#<skillId>` / `POSTING#<id>` |
| Skill (canonical) | `SKILL#<skillId>` | `META` | `CAT#<category>` / `SKILL#<skillId>` | — |
| Aggregate counter | `STAT#<roleFamily>` | `SKILL#<skillId>` | — | — |
| Unknown term (review) | `REVIEW#PENDING` | `TERM#<normalized>` | — | — |
| Ingest run | `RUN#<yyyy-mm-dd>` | `SOURCE#<connector>` | — | — |
| Dedup marker | `DEDUP#<sha256>` | `META` | — | — |
| User saved posting | `USER#<cognitoSub>` | `SAVED#<postingId>` | — | — |

**GSI1** — `GSI1PK`/`GSI1SK`, projection ALL. Serves: postings by company (date-sorted), companies by tier.
**GSI2** — `GSI2PK`/`GSI2SK`, projection KEYS_ONLY + `title`, `companySlug`. Serves: postings by role family (date-sorted), postings mentioning a skill.

### Key field definitions

**Posting**
```go
type Posting struct {
    ID           string    // sha256(canonicalURL)[:16]
    CompanySlug  string
    Title        string
    RoleFamily   string    // build_release | liveops_backend | platform_infra |
                           // gameplay | tools | security | other
    Location     string
    Remote       bool
    URL          string
    Source       string    // greenhouse | lever | ashby | workday | scraper | manual
    PostedAt     time.Time
    FirstSeenAt  time.Time
    LastSeenAt   time.Time
    ClosedAt     *time.Time
    RawS3Key     string    // s3://.../raw/<source>/<id>.json
    ContentHash  string    // sha256 of normalized text — drives re-extraction
    ExtractVer   int       // dictionary+prompt version used
    SkillCount   int
}
```

**Posting→Skill edge**
```go
type PostingSkill struct {
    PostingID  string
    SkillID    string
    Required   bool    // from "Requirements" vs "Nice to have" section
    Evidence   string  // ≤200 chars, the matched span — for debugging & UI
    Confidence float32 // 1.0 dictionary, 0.0–1.0 from Bedrock
    Method     string  // dict | llm
}
```

**Skill (canonical)**
```go
type Skill struct {
    ID          string   // slug: "perforce", "unreal-build-tool"
    Display     string   // "Perforce (Helix Core)"
    Category    string   // see §6 taxonomy
    Aliases     []string // matched case-insensitively, word-boundary
    Patterns    []string // optional regex for tricky cases
    Deprecated  bool
    MergedInto  string   // if this skill was folded into another
}
```

### The aggregation problem (read this carefully)

DynamoDB cannot do `GROUP BY skill COUNT(*)`. This is the single most important design constraint in the project. Two mechanisms, both required:

**1. Write-time counters.** When a `POSTING#<id> / SKILL#<sid>` edge is written, transactionally `ADD` 1 to `STAT#<roleFamily> / SKILL#<sid>`. Use `TransactWriteItems` with a `ConditionExpression: attribute_not_exists(PK)` on the edge — if the edge already exists the transaction fails and the counter is not double-incremented. This makes the operation idempotent, which matters because SQS is at-least-once delivery and your worker *will* process the same message twice.

**2. Nightly rollup reconciliation.** `cmd/rollup` scans GSI2 by skill, recounts, and corrects drift. Counters drift — from failed transactions, from deleted postings, from schema changes. A system that only has mechanism 1 will silently produce wrong numbers, and wrong numbers here mean you study the wrong things. Build the rollup in the same phase as the counters, not later.

Also track a "denominator": `STAT#<roleFamily> / __TOTAL__` holding the count of active postings in that family, so the UI can show *"Perforce — 78% of build/release postings"* rather than a meaningless raw count.

**Posting lifecycle.** A posting disappearing from an ATS feed means it closed. Do not delete it — set `ClosedAt`, exclude from active stats, keep for historical trend. "Perforce demand rose from 61% to 78% over six months" is a much better portfolio finding than a static snapshot.

---

## 5. Ingestion

Three tiers, in strict priority order. Always prefer the highest tier available for a given company.

### Tier 1 — ATS public JSON APIs (target: 65–75% of companies)

Most studios outsource their careers page to an applicant tracking system, and the major ATSs expose public, documented, unauthenticated JSON endpoints intended for consumption. This is the clean path: no browser, no parsing HTML, stable schemas, and no ambiguity about whether you're welcome.

Connectors to implement, in this order:

| ATS | Endpoint shape | Notes |
|---|---|---|
| Greenhouse | `boards-api.greenhouse.io/v1/boards/{token}/jobs?content=true` | Most common in games. `content=true` returns full HTML body |
| Lever | `api.lever.co/v0/postings/{company}?mode=json` | Returns structured `lists` (requirements as arrays) — best data quality of any source |
| Ashby | `api.ashbyhq.com/posting-api/job-board/{name}` | Growing fast among newer studios |
| SmartRecruiters | `api.smartrecruiters.com/v1/companies/{id}/postings` | Two-call: list then detail |
| Workable | `apply.workable.com/api/v1/widget/accounts/{token}` | Smaller studios |
| Workday | `{tenant}.wd*.myworkdayjobs.com/wday/cxs/{tenant}/{site}/jobs` (POST) | Undocumented but consistent JSON. Big publishers (EA, Take-Two, Ubisoft-scale) live here. **Try this before reaching for the scraper** — it will save you enormous pain |

**Connector interface:**
```go
type Connector interface {
    Name() string
    Fetch(ctx context.Context, cfg CompanyConfig) ([]RawPosting, error)
}

type RawPosting struct {
    ExternalID string
    URL        string
    Title      string
    Location   string
    PostedAt   time.Time
    BodyHTML   string
    Structured map[string]any // Lever's lists, Greenhouse's metadata, etc.
}
```

Register connectors in a map keyed by ATS name. Adding a company should be a data change (`data/companies.yaml`), never a code change.

**Company registry** (`data/companies.yaml`, checked into the repo):
```yaml
- slug: epic-games
  name: Epic Games
  tier: platform          # platform | aaa | midsize | indie | tools
  ats: greenhouse
  token: <board-token>
  roleFilters: ["build", "release", "infrastructure", "platform", "devops", "live"]
```

Finding each company's ATS is a manual research task: open the careers page, watch the network tab, note the XHR domain. Budget a couple of hours for 40 companies. **This is the highest-leverage work in the project** — no amount of clever extraction compensates for a thin company list. Seed candidates in §14.

### Tier 2 — Headless scraper (target: ~20%)

`cmd/scraper` using **chromedp** (pure Go, drives headless Chrome via CDP — no Node dependency). Runs as an on-demand Fargate task consuming from `ingest-queue`. Needs ~2 vCPU / 4 GB; Chrome is heavy.

Rules, non-negotiable:
- Fetch `robots.txt` first and honor it. Cache per-domain for 24h.
- Identifying User-Agent with a contact URL. Do not pretend to be a browser you aren't.
- Max 1 request per 5 seconds per domain, jittered. Global concurrency cap of 3 domains.
- Conditional requests (`ETag` / `If-Modified-Since`) where supported.
- Hard timeout 30s per page; on any 4xx, mark the company `scrapeDisabled: true` and stop. Do not retry into a wall.
- Public pages only. No login walls, no CAPTCHA solving, no proxy rotation. If a site doesn't want automated access, that's an answer — drop the company or use manual submission.

### Tier 3 — Manual submission

`POST /v1/postings/submit {url}`. Enqueues to `ingest-queue`; the scraper task fetches and normalizes it. This is the escape hatch for the one-off posting you see on LinkedIn or in a Discord, and it's why the share-sheet extension exists on mobile (§7).

**SSRF is a real vulnerability here and you should treat it as one.** See §10.

### Deduplication

Canonicalize the URL (strip UTM and tracking params, lowercase host, drop fragment), `sha256` it, that's the `PostingID`. Also write a `DEDUP#<contentHash>` marker with a 30-day TTL so the same job cross-posted under two URLs collapses to one. Set `ContentHash` from the normalized body text; if it changes on a re-crawl, the posting was edited — re-run extraction.

### Scheduling

EventBridge Scheduler → `ecs:RunTask` on `cmd/ingest`, daily 06:00 UTC. One run walks the full company registry, respecting per-connector rate limits. Expect 5–15 minutes for 40 companies. `cmd/rollup` at 07:00 UTC.

Jenkins additionally gets a parameterized **on-demand backfill job** so you can re-ingest a single company without waiting for the schedule. Use EventBridge for the recurring path — a Jenkins cron would work but makes the pipeline responsible for data-plane concerns, which is the wrong separation.

---

## 6. Extraction Pipeline

Five stages in `cmd/worker`, consuming `extract-queue`.

### Stage 1 — Normalize

HTML → clean text via `goquery`. Strip nav/footer/script/style. Preserve list structure (`<li>` → `- ` lines) and headings (`<h*>` → `## `) — that structure is what Stage 2 depends on, so don't flatten it. Collapse whitespace. Where the ATS already gives structured data (Lever's `lists` array is the standout), use it directly and skip the HTML path.

### Stage 2 — Segment

Classify each block into `responsibilities | requirements | nice_to_have | benefits | boilerplate` using heading heuristics:

- `requirements` ← "requirements", "qualifications", "what you'll need", "you have", "minimum", "basic qualifications"
- `nice_to_have` ← "nice to have", "bonus", "preferred", "plus", "pluses", "even better", "preferred qualifications"
- `responsibilities` ← "responsibilities", "what you'll do", "the role", "about the role"
- `benefits` ← "benefits", "perks", "compensation", "we offer", "equal opportunity", "EEO"

Everything after a `benefits` heading is boilerplate — discard. This matters more than it sounds: DEI statements and benefits blurbs are the single biggest source of extraction noise, and they're long.

**The `requirements` vs `nice_to_have` split is the most valuable signal in the entire system.** "Perforce is required at 40% of studios" and "Perforce is nice-to-have at 40% of studios" imply completely different study priorities. Make sure the UI never collapses them.

### Stage 3 — Dictionary pass

`data/skills.yaml` — canonical skills with aliases. Word-boundary, case-insensitive matching; optional regex for tricky cases (`C\+\+`, `\.NET`, `C#` all need escaping care, and `Go` needs a context guard or it matches every "go to" in the document).

Taxonomy — categories, with seed entries. Expect to reach 250–400 skills by month 3.

| Category | Seeds |
|---|---|
| `vcs` | Perforce/Helix Core, Git, Git LFS, Plastic SCM, Subversion, Diversion |
| `build_system` | Unreal Build Tool, BuildGraph, UAT, Unreal Horde, Bazel, CMake, MSBuild, SCons, FASTBuild, Incredibuild, SN-DBS |
| `ci_cd` | Jenkins, TeamCity, GitHub Actions, GitLab CI, Buildkite, CircleCI, Azure DevOps, Bamboo |
| `cloud` | AWS, GCP, Azure, EC2, S3, Lambda, ECS, EKS, GameLift, CloudFormation, Terraform, Pulumi, Ansible |
| `containers` | Docker, Kubernetes, Agones, Helm, containerd, Nomad |
| `languages` | C++, C#, Python, Go, Rust, Bash, PowerShell, TypeScript, Lua, Perl |
| `engines` | Unreal Engine, Unity, Godot, proprietary/in-house engine |
| `platforms` | PS5, Xbox Series, Nintendo Switch, Steam/SteamPipe, Epic Games Store, iOS, Android, console certification/TRC/TCR/lotcheck |
| `liveops` | PlayFab, Pragma, Nakama, AccelByte, Beamable, Firebase, matchmaking, telemetry, remote config, feature flags, A/B testing |
| `data` | Postgres, MySQL, Redis, DynamoDB, Kafka, Kinesis, Snowflake, BigQuery, Databricks, Spark, Airflow |
| `observability` | Datadog, Grafana, Prometheus, Splunk, New Relic, Sentry, OpenTelemetry, crash symbolication, Backtrace |
| `security` | IAM, secrets management, code signing, SAST/DAST, anti-cheat, supply chain, SOC 2, threat modeling |
| `build_infra` | DDC (derived data cache), build farm, artifact storage, Artifactory, symbol server, asset validation, cook/package pipeline |
| `practices` | Agile, Scrum, on-call, incident response, SRE, postmortems, code review, mentoring |
| `meta` | years of experience, degree requirement, shipped-title requirement, on-site/hybrid |

`meta` is worth special handling — extract "5+ years" and "shipped at least one AAA title" as structured facts, not skills. Knowing that 70% of postings demand a shipped title changes your strategy far more than knowing they want Python.

### Stage 4 — Bedrock fallback

Only for requirement/nice-to-have bullets with **zero** dictionary hits. This keeps cost and latency near zero on the common path.

- Model: `anthropic.claude-3-5-haiku` for volume; escalate to Sonnet if precision on the validation set is short of target.
- Batch 20 unmatched bullets per call.
- Temperature 0. Prompt for strict JSON: `[{term, category, isRequired, evidence, confidence}]`.
- **Validate every response against a JSON schema.** Reject and log on mismatch. Never trust the shape.
- Cache by `sha256(bullet_text)` in DynamoDB with a 90-day TTL. Re-running extraction across the corpus should cost ~nothing the second time.

Estimated cost: ~500 postings × ~3 unmatched bullets, batched, ≈ 75 Haiku calls at first full run, pennies. Cache makes subsequent runs free. If you're spending more than $2/month here, something is wrong — most likely the dictionary is too thin or segmentation is leaking boilerplate.

### Stage 5 — Review queue

Bedrock-discovered terms do **not** go straight into the canonical skill list. They land as `REVIEW#PENDING / TERM#<normalized>` with occurrence count and example evidence. You triage them in the app (§7): **create** as a new canonical skill, **alias** into an existing one, or **reject** as noise.

This is the mechanism that makes the system get better over time rather than accumulating garbage. Approved terms are written back to `data/skills.yaml` via an API endpoint that opens a commit — meaning the dictionary is version-controlled, reviewable, and the LLM never silently mutates your taxonomy. Every triage decision shrinks the set of bullets that need Bedrock next run.

### Re-extraction

Stamp `ExtractVer` on every posting. Bump it whenever `skills.yaml` or the prompt changes materially. A Jenkins job re-enqueues all postings where `ExtractVer < current`. Because the Bedrock cache is keyed on bullet text rather than version, re-extraction is fast and nearly free.

### Validation

Hand-label 50 postings (a couple of hours). Store as `testdata/labeled/*.json`. `go test ./internal/extract` computes precision/recall against them and **fails the build below 0.90 precision**. Without this you have no idea whether the numbers driving your study plan are real. Do this in Phase 1, not Phase 8.

---

## 7. API Surface

REST, JSON, versioned under `/v1`. Cognito JWT authorizer at API Gateway; `admin` scope for writes, `read` for queries.

| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/skills` | **The money endpoint.** Ranked frequency. Params: `roleFamily`, `tier`, `required` (true/false/both), `since`, `limit` |
| GET | `/v1/skills/{id}` | Detail + trend series + example evidence |
| GET | `/v1/skills/{id}/postings` | Postings mentioning this skill |
| GET | `/v1/postings` | Params: `company`, `roleFamily`, `since`, `cursor`, `limit` |
| GET | `/v1/postings/{id}` | Detail incl. extracted skills w/ evidence |
| POST | `/v1/postings/submit` | `{url}` → 202, enqueues |
| GET | `/v1/companies` | Registry + posting counts + last ingest status |
| GET | `/v1/reviews` | Pending unknown terms, sorted by frequency |
| POST | `/v1/reviews/{term}` | `{action: create|alias|reject, targetSkillId?, category?}` |
| POST | `/v1/exports` | `{format: csv\|xlsx, filters}` → 202 + `exportId` |
| GET | `/v1/exports/{id}` | Status; when ready, presigned S3 URL (15 min TTL) |
| GET | `/v1/stats/overview` | Corpus size, companies, last run, coverage |
| GET | `/healthz`, `/readyz` | ALB health checks — unauthenticated, no VPC egress |

**Conventions:** cursor pagination (opaque base64 of the DynamoDB `LastEvaluatedKey`, never offset). RFC 7807 problem+json errors. `X-Request-ID` echoed and logged. Router: `chi` (stdlib-compatible, minimal).

---

## 8. Client (Expo)

One Expo codebase → iOS, Android, web. Expo Router for file-based routing.

### Screens

| Route | Content |
|---|---|
| `/(auth)/sign-in` | Cognito Hosted UI via `expo-auth-session`, PKCE |
| `/(tabs)/syllabus` | **Home.** Ranked skill list, % of postings, required-vs-preferred toggle, role-family filter, category grouping |
| `/(tabs)/syllabus/[skillId]` | Trend chart, evidence snippets, companies demanding it |
| `/(tabs)/postings` | Filterable list, infinite scroll |
| `/(tabs)/postings/[id]` | Detail, extracted skills w/ highlighted evidence, link out |
| `/(tabs)/review` | Unknown-term triage queue. Swipe or tap: create / alias / reject |
| `/(tabs)/companies` | Registry, coverage, ingest health |
| `/submit` | Paste URL, or target of the share extension |
| `/export` | Filter builder → CSV/XLSX → share sheet or download |

**The review queue is the screen that justifies mobile existing.** Triaging unknown terms is inherently bite-sized, and doing it on a phone in ten-minute gaps is meaningfully better than doing it at a desk. Design it for one-handed use.

### Libraries

`expo-router`, `@tanstack/react-query` (server state; do not add Redux), `expo-auth-session` + `expo-secure-store` (PKCE + token storage — never AsyncStorage for tokens), `nativewind` (Tailwind across native and web), `victory-native` + `react-native-svg` (charts that render on all three targets), `expo-share-intent` (share extension).

### Share extension

Requires native modules, so **EAS dev builds — Expo Go will not work.** iOS Share Extension target + Android `ACTION_SEND` intent filter, added via config plugin. Flow: Safari/Chrome → Share → Job Syllabus → app opens on `/submit` prefilled → POST → 202 → toast. Build this in Phase 7, not earlier; it's the fiddliest client work and it's not on the critical path.

### Web

`npx expo export -p web` → static bundle → S3 → CloudFront (OAC, no public bucket). SPA fallback: CloudFront error responses map 403/404 → `/index.html` with 200. Custom domain optional (Route 53 + ACM in `us-east-1` for CloudFront).

### Auth flow

Cognito user pool, hosted UI, authorization-code + PKCE. Tokens in `expo-secure-store` (Keychain/Keystore) on native; on web, memory + refresh-token cookie. Refresh on 401, single retry, then bounce to sign-in. Attach the ID token as `Authorization: Bearer`.

---

## 9. Infrastructure (Terraform)

### Layout

```
infra/terraform/
  bootstrap/          # run once, locally: S3 state bucket + DynamoDB lock table
  modules/
    network/          # VPC, subnets, endpoints, SGs
    data/             # DynamoDB table + GSIs, S3 buckets
    ecr/              # 5 repos w/ lifecycle policies + scan-on-push
    ecs-cluster/      # cluster, capacity providers, exec role
    service-api/      # task def, service, ALB target group, autoscaling
    service-worker/   # queue-depth-driven autoscaling
    task-scheduled/   # reusable: EventBridge Scheduler → RunTask
    queues/           # SQS + DLQs + redrive policies
    auth/             # Cognito pool, client, domain
    api-gateway/      # HTTP API, VPC Link, JWT authorizer, custom domain
    web/              # S3 + CloudFront + OAC
    jenkins/          # EC2, EBS, SG, IAM, ALB
    observability/    # log groups, alarms, dashboard, SNS
  envs/
    dev/              # main.tf, terraform.tfvars, backend.tf
    prod/
```

Separate env directories over workspaces — explicit, greppable, and easier to explain in an interview.

### Bootstrap chicken-and-egg

`bootstrap/` has a **local** backend and creates the S3 state bucket + DynamoDB lock table. Run it once by hand, commit the resulting bucket name into `envs/*/backend.tf`, then everything else uses remote state. Document this in `docs/bootstrap.md`; it's the first thing a reviewer will trip over.

### Network and the NAT Gateway decision

2 AZs. Public subnets for ALB + Jenkins. Private subnets for Fargate tasks.

**NAT Gateway costs ~$32/month — the single largest line item, larger than all compute combined.** Options:

1. **NAT Gateway.** Correct, conventional, $32/mo.
2. **VPC endpoints, no NAT.** Gateway endpoints for S3 and DynamoDB are free. Interface endpoints for ECR API, ECR DKR, CloudWatch Logs, SQS, Secrets Manager, Bedrock run ~$7/mo each — six of them exceeds the NAT. Worse.
3. **Fargate tasks in public subnets with public IPs, security groups closed to inbound, plus free gateway endpoints for S3/DynamoDB.** ~$0/mo.

**Recommendation: option 3 for `dev`, option 1 for `prod`.** Then write it up. "I chose public-subnet Fargate in dev to eliminate a $32/month NAT Gateway, and here is the security tradeoff I accepted and why it's acceptable for that environment" is a genuinely good interview answer — it shows you read the bill, which most engineers don't.

Note that the scraper needs general internet egress by definition, so it can't be endpoint-only regardless.

### Sizing

| Service | Size | Scaling |
|---|---|---|
| `api` | 0.25 vCPU / 512 MB | 1–3 tasks, CPU > 70% |
| `worker` | 0.5 vCPU / 1 GB | 0–4 tasks, SQS `ApproximateNumberOfMessagesVisible` |
| `scraper` | 2 vCPU / 4 GB | On-demand RunTask only (Chrome is hungry) |
| `ingest` | 0.5 vCPU / 1 GB | Scheduled RunTask only |
| Jenkins | t4g.small (ARM) | Single instance |

Fargate Spot for `worker` and `scraper` — both are interruption-tolerant, and it's ~70% cheaper.

### Observability

CloudWatch Logs (30-day retention; unbounded retention is a classic surprise bill). Structured JSON logs via `log/slog`. OpenTelemetry → ADOT sidecar → CloudWatch. Alarms → SNS → email: DLQ depth > 0, ingest task failure, API 5xx > 1%, Bedrock throttling, monthly spend > $50.

**DLQ depth > 0 is the one alarm that will actually save you.** Everything else is nice to have.

---

## 10. Jenkins

You chose Jenkins over GitHub Actions deliberately. Do it properly or the choice isn't worth its cost.

### Controller

`t4g.small` (ARM Graviton, ~$12/mo), Amazon Linux 2023, Jenkins LTS. Separate 20 GB gp3 EBS volume for `JENKINS_HOME`, mounted at `/var/lib/jenkins`, DLM snapshot policy daily with 7-day retention — so a controller rebuild is `terraform apply` + volume attach.

Behind an ALB with ACM TLS. Security group restricted to your IP or a Tailscale subnet router. **Do not expose Jenkins to the open internet.** Attacks on exposed Jenkins controllers are automated and constant.

IAM **instance profile** with permissions for ECR push, ECS deploy, S3, and Terraform. **Zero static AWS access keys anywhere in Jenkins.** This is the DevSecOps point of the whole exercise — say it explicitly in the writeup.

### Configuration as Code (the part that matters)

`ci/jenkins.yaml` (JCasC) + `ci/plugins.txt`, both in the repo. Plugins installed from the manifest at provision time via `jenkins-plugin-cli` in EC2 user data. Job definitions via **Job DSL** seed job, itself defined in JCasC.

**Success criterion: terminate the EC2 instance, `terraform apply`, and get a fully-configured Jenkins with all jobs present, with no clicking.** Test this at least once, deliberately, and record it. That demonstration is worth more than the jobs themselves — most people who list Jenkins on a résumé have never done it.

### Agents

Amazon ECS plugin → **ephemeral Fargate agents**. Zero idle cost, clean workspace per build, no snowflake agent drift. Agent image: `ci/agent.Dockerfile` with Go, Docker CLI, Terraform, Node, `eas-cli`, Trivy, `awscli`.

Docker-in-Docker on Fargate is painful. Use **Kaniko** or `docker buildx` with a remote BuildKit builder for image builds instead of privileged DinD.

### Pipelines

| Job | Trigger | Stages |
|---|---|---|
| `api-build` | push to `main`, path `cmd/**`, `internal/**` | `go vet` → `golangci-lint` → `go test -race -cover` → extraction validation gate → build (multi-stage → distroless) → Trivy (fail on HIGH+) → ECR push tagged `$GIT_SHA` → `ecs update-service` → poll stability → smoke test `/healthz` |
| `infra-plan` | PR touching `infra/**` | `fmt -check` → `validate` → `tfsec` → `checkov` → `plan` → post plan as artifact |
| `infra-apply` | manual, after `infra-plan` | **input approval** → `apply` |
| `client-build` | tag `client-v*` | `tsc --noEmit` → lint → jest → `eas build --platform all` → `expo export -p web` → S3 sync → CloudFront invalidation |
| `backfill` | manual, parameterized `COMPANY` | RunTask `ingest` scoped to one company |
| `reextract` | manual, parameterized `EXTRACT_VER` | Enqueue all postings below version |

Deploy strategy: ECS rolling with `minimumHealthyPercent: 100`, `maximumPercent: 200`, circuit breaker with automatic rollback enabled. Blue/green via CodeDeploy is a reasonable v2 upgrade and a good talking point.

### Cost mitigation

Controller is ~$12/mo. If that matters, an EventBridge schedule stopping the instance nights/weekends roughly halves it — and "I automated my own build server's cost profile" is itself a nice detail. Don't bother until the bill annoys you.

---

## 11. Security

Your differentiator is being a security-minded release engineer. This section is where that shows, so treat it as a feature rather than a checklist.

### SSRF on manual URL submission (highest-severity issue in the design)

`POST /v1/postings/submit` takes a user-supplied URL and fetches it server-side from inside your VPC. Unmitigated, that is a textbook SSRF into the instance metadata service.

Required controls:
- Allowlist schemes: `https` only.
- Resolve DNS **first**, then validate the resolved IPs against a denylist: `169.254.0.0/16` (IMDS), `10/8`, `172.16/12`, `192.168/16`, `127/8`, `::1`, `fc00::/7`, `0.0.0.0/8`.
- **Re-validate after every redirect** — DNS rebinding defeats a single up-front check. Cap redirects at 3.
- Custom `net.Dialer` with a `Control` hook that rejects private destinations at connect time. This is the control that actually holds; the pre-checks are defense in depth.
- Response size cap 5 MB, timeout 15s.
- IMDSv2 enforced (`http_tokens = "required"`) on the Jenkins EC2 instance.
- Rate limit: 20 submissions per user per hour.

Write this up as `docs/security-ssrf.md` with the reasoning. It's a strong interview artifact.

### Prompt injection

Posting text is untrusted input flowing into Bedrock. A posting could contain instructions aimed at your extractor. Mitigations: model output is parsed as data only and never executed or used to build queries; strict JSON-schema validation with rejection on mismatch; the review queue means no LLM-discovered term enters the taxonomy without your approval; log and alert on schema-validation failures — a spike is a signal worth looking at.

The Bedrock task role gets `bedrock:InvokeModel` on specific model ARNs only.

### Baseline

Distroless or `scratch` base images, non-root, read-only root filesystem, no shell in the runtime image. ECR scan-on-push plus Trivy gating in the pipeline. Least-privilege task roles scoped to specific table/queue/bucket ARNs — no wildcards. SSM Parameter Store (SecureString) over Secrets Manager for the handful of ATS tokens ($0 vs $0.40/secret/month). Cognito: MFA on, password policy, advanced security if the tier allows. S3: all public access blocked, SSE-S3 minimum, CloudFront OAC for the web bucket. Dependabot or Renovate on Go modules and npm.

---

## 12. Cost Estimate

| Item | Monthly |
|---|---|
| Fargate `api` (1 task, 0.25/512, always on) | ~$9 |
| Fargate `worker` (Spot, ~2h/day) | ~$1 |
| Fargate `scraper` + `ingest` (scheduled) | ~$2 |
| Jenkins EC2 t4g.small + 20 GB gp3 | ~$14 |
| ALB (internal) | ~$16 |
| API Gateway HTTP API | <$1 |
| DynamoDB on-demand | ~$1 |
| S3 + CloudFront | ~$1 |
| Bedrock (Haiku, cached) | <$1 |
| Cognito (free tier) | $0 |
| NAT Gateway — **dev only, if used** | ~$32 |
| **Total without NAT** | **~$45** |

Slightly over the $40 goal, and the ALB is the reason. Options if it matters: drop the internal ALB and put API Gateway directly in front of Fargate via Cloud Map service discovery + VPC Link (saves $16, slightly less conventional); or run `api` on Fargate Spot too (saves ~$6, accepts occasional interruption). **Set an AWS Budget alarm at $60 before Phase 2**, not after.

---

## 13. Phased Build Plan

Each phase has a definition of done. Do not proceed until it passes.

### Phase 0 — Scaffold (½ day)
Monorepo layout (§14), Go module, `Makefile`, `golangci-lint` config, `docker-compose.yml` with DynamoDB Local + LocalStack, `.editorconfig`, `docs/`.
**DoD:** `make test` and `make lint` pass on an empty test suite.

### Phase 1 — Local vertical slice (3–4 days) ← *the most important phase*
No cloud. No auth. No Terraform. Go binary + DynamoDB Local. One Greenhouse connector, one Lever connector, normalize + segment + dictionary extraction, seed `skills.yaml` (~80 skills), CLI that ingests 5 companies and prints the ranked table. Hand-label 50 postings; build the validation test.

**DoD:** `make ingest && make report` prints a ranked skill table from real postings. Validation test ≥0.90 precision.

Getting real data on day three is what keeps this project alive. If you build infrastructure first, you will spend three weeks on Terraform and never learn whether the extraction works — and extraction is the part the whole thing rests on. If precision comes in at 0.6, you want to know now, while the fix is cheap.

### Phase 2 — Terraform baseline + Jenkins (4–5 days) ← *the slog*
Bootstrap state. `network`, `data`, `ecr`, `ecs-cluster`, `queues`, `observability` modules. Jenkins module: EC2 + EBS + ALB + IAM + JCasC + plugins + Job DSL seed. Fargate agent image. `api-build` and `infra-plan`/`infra-apply` pipelines.
**DoD:** Terminate the Jenkins instance; `terraform apply`; Jenkins returns fully configured with all jobs, no clicking. Record it.

Budget the most calendar time here and expect frustration. Everything after this is easier because of it.

### Phase 3 — Deploy API (2–3 days)
`service-api`, ALB, API Gateway HTTP API + VPC Link. Real DynamoDB. Read endpoints only, no auth yet (locked to your IP).
**DoD:** `curl` your API Gateway URL, get ranked skills from real data, deployed by Jenkins from a git push.

### Phase 4 — Ingestion at scale (3–4 days)
All Tier-1 connectors. Company registry to 40+. SQS queues, `service-worker` with queue-depth autoscaling, EventBridge schedules, `cmd/rollup`, dedup, lifecycle handling.
**DoD:** 500+ postings, 40+ companies, daily runs green, DLQ empty, counters match rollup.

### Phase 5 — Bedrock + review queue (2–3 days)
Bedrock fallback, response caching, review-queue items, review API, `skills.yaml` writeback, re-extraction job.
**DoD:** Unknown terms surface in `GET /v1/reviews`; approving one via API updates the dictionary and re-extraction picks it up.

### Phase 6 — Auth + client (4–5 days)
Cognito module. Expo app: sign-in, syllabus, skill detail, postings, review queue. S3 + CloudFront web deploy. `client-build` pipeline.
**DoD:** Sign in and browse the syllabus on iOS, Android, and web.

### Phase 7 — Scraper + share extension (3–4 days)
`cmd/scraper` with chromedp, robots.txt, rate limiting, SSRF controls. Manual submit endpoint. Workday connector. Share extension via EAS dev build.
**DoD:** Share a posting from mobile Safari into the app; it appears extracted within two minutes.

### Phase 8 — Export, trends, polish, writeup (3–4 days)
CSV/XLSX export via presigned URLs. Trend charts. Coverage dashboard. Alarms. `docs/architecture.md`, `docs/security-ssrf.md`, `docs/cost.md`, phase writeups, README with diagram.
**DoD:** Export the syllabus spreadsheet. Publish the writeup. **Use the spreadsheet to revise your study plan** — that was the point.

**Total: roughly 6–8 weeks at 10 hrs/week**, which lines up with the roadmap's Project 2/3 window and can run in parallel with Perforce and Unreal study.

---

## 14. Repository Layout

```
job-syllabus/
├── cmd/{api,ingest,scraper,worker,rollup}/main.go
├── internal/
│   ├── api/          # chi router, handlers, middleware, problem+json
│   ├── auth/         # JWT validation, scopes
│   ├── connectors/   # greenhouse.go, lever.go, ashby.go, workday.go, ...
│   ├── extract/      # normalize.go, segment.go, dictionary.go, bedrock.go
│   ├── model/        # domain types
│   ├── store/        # DynamoDB access, single-table encoding
│   ├── queue/        # SQS producer/consumer
│   ├── export/       # CSV/XLSX generation
│   ├── safefetch/    # SSRF-hardened HTTP client
│   └── config/       # env + SSM config loading
├── data/
│   ├── companies.yaml
│   └── skills.yaml
├── testdata/labeled/ # 50 hand-labeled postings
├── mobile/           # Expo app
├── infra/terraform/  # see §9
├── ci/
│   ├── jenkins.yaml      # JCasC
│   ├── plugins.txt
│   ├── jobs.groovy       # Job DSL seed
│   ├── agent.Dockerfile
│   └── Jenkinsfile.*
├── docs/
├── Dockerfile        # multi-stage, one arg selects binary
├── docker-compose.yml
└── Makefile
```

**Approved Go dependencies:** `aws-sdk-go-v2`, `go-chi/chi`, `PuerkitoBio/goquery`, `chromedp/chromedp`, `xuri/excelize`, `goccy/go-yaml`. Anything else needs justification.

---

## 15. Seed Company List

Per the roadmap's guidance, weight toward **game-adjacent platform companies** — they hire on conventional software interviews, pay closer to normal tech scale, and post more of exactly the roles you're targeting.

**Platform / tools (highest priority):** Epic Games, Unity, Roblox, Discord, Valve, Riot Games, AWS GameTech, Microsoft Gaming/Xbox, Sony PlayStation (platform teams), Nintendo of America, Improbable, Pragma, Hathora, AccelByte, Perforce, JetBrains, Incredibuild, Backtrace/Sauce Labs.

**AAA / large:** EA, Activision Blizzard, Ubisoft, Take-Two/Rockstar/2K, Bungie, Naughty Dog, Insomniac, 343/Halo Studios, id Software, Bethesda, CD Projekt, Larian, Remedy, Embark, Sucker Punch.

**Mid / live-service:** Behaviour, Digital Extremes, Bohemia, Hi-Rez, Gearbox, Respawn, Sledgehammer, Treyarch, Certain Affinity, Jagex, Wargaming, Scopely, Zynga, King, Supercell, Niantic, Skydance.

For each: open the careers page, watch the network tab, record ATS + token in `companies.yaml`. **Budget ~2 hours for the first 40. This is the highest-leverage two hours in the project** — extraction quality is capped by corpus quality.

Also track `tier` per company so you can slice the syllabus by employer type. The build-engineer skill profile at Epic and at a 30-person indie are meaningfully different, and you're targeting the former.

---

## 16. Open Questions

1. **Role-family classification** is currently title-keyword based. Titles are inconsistent across studios ("Build Engineer" vs "Release Engineer" vs "Infrastructure Engineer, Game Services"). Consider a Bedrock classification pass in Phase 5 if precision is poor. Measure before adding it.
2. **Salary data** is inconsistently present (CA/CO/NY/WA disclosure laws mean coverage is decent but partial). Worth extracting where available; don't build a feature on it.
3. **Historical trend** needs several months of data to be interesting. Start collecting in Phase 4 even though nothing consumes it until Phase 8.
4. **Guest read access** — sharing a public link to the syllabus makes it a much better portfolio piece. Adds a public unauthenticated path with caching and abuse considerations. Deferred; revisit at Phase 8.
5. **v2 candidates:** application tracker; resume gap-analysis against the syllabus; weekly email digest; Perforce-vs-Git trend as a standalone blog post.

---

## 17. First Prompt for Claude Code

> Read `docs/design.md`. Implement **Phase 0** and **Phase 1** only.
>
> Start with the repo scaffold from §14, then the local vertical slice: Greenhouse and Lever connectors, HTML normalization, section segmentation, dictionary-based extraction with a seed `skills.yaml` covering the categories in §6, DynamoDB Local persistence using the single-table design in §4, and a CLI that ingests a company list and prints a ranked skill-frequency table.
>
> Do not write any Terraform, Jenkins configuration, auth, or Expo code yet.
>
> Before writing connector code, show me the `Connector` interface and the `companies.yaml` schema for review. Before writing the extraction pipeline, show me the proposed section-segmentation heuristics.
>
> Definition of done: `make ingest && make report` prints a ranked table from real postings for at least 5 companies.
