# Phase 5 — Bedrock + review queue

**Status: done, DoD validated against real production traffic — not just
locally.** Stage 4 (Bedrock fallback for zero-dictionary-hit
requirement/nice-to-have bullets) and Stage 5 (review queue) are both live.
Unmatched bullets are batched to Claude Haiku, validated against a strict
JSON schema, and cached by `sha256(bullet_text)` with a 90-day TTL. A
Bedrock finding that already matches a known skill becomes a `PostingSkill`
edge directly; anything genuinely new lands in `GET /v1/reviews` with an
occurrence count and evidence. `POST /v1/reviews/{term}` triages
create/alias/reject. `cmd/rollup reextract` re-enqueues the corpus after an
`extract.Version` bump so an approval's effect reaches postings that were
already processed under the old dictionary.

## DoD proof

Rockstar Games' real "Animation Tools Programmer" posting has a bullet
reading `"Experience with MotionBuilder and/or Maya."` — neither term was
in the dictionary, so it went to Bedrock, which correctly identified both
as skill terms; neither matched any known skill, so both surfaced in the
review queue:

```
$ curl "$API_URL/v1/reviews"
{"reviews":[{"term":"maya","category":"other","occurrences":4,
  "evidence":["Experience with MotionBuilder and/or Maya.", ...]},
 {"term":"motionbuilder","category":"other","occurrences":4, ...}, ...]}
```

Approving `maya` via the API:

```
$ curl -X POST "$API_URL/v1/reviews/maya" -d '{"action":"create","category":"engines","display":"Autodesk Maya"}'
{"term":"maya","action":"create","skill":{"id":"maya","display":"Autodesk Maya","category":"engines",...}}
```

...then triggering `backfill` for `rockstar-games` (re-enqueuing its
postings for re-extraction) shows the same posting now matching `maya`
**via the dictionary, not Bedrock** — the approval took effect immediately,
no redeploy:

```
$ curl "$API_URL/v1/postings/34338f3fc0d602b6"
{"skills":[
  {"skillId":"cpp","method":"dict",...},
  {"skillId":"csharp","method":"dict",...},
  {"skillId":"maya","display":"Autodesk Maya","method":"dict",
   "evidence":"- Experience with MotionBuilder and/or Maya."},
  {"skillId":"motionbuilder","method":"dict",...}
]}
```

DLQ stayed empty throughout (`ApproximateNumberOfMessages: 0`) — including
during the window where every real Bedrock call was failing (see bug #4
below), confirming Stage 4's designed graceful degradation: a Bedrock
outage doesn't lose or dead-letter a single posting, it just skips the
LLM enhancement for that pass.

All three triage actions were exercised end to end: `create` against
production (`maya`, `motionbuilder`, above), `reject` against production
(a genuinely vague Bedrock finding, `"cloud technologies"`, dismissed as
noise and confirmed gone from `GET /v1/reviews` on the next call), and
`alias` locally against DynamoDB Local (merging a paraphrase into
`perforce`'s existing alias list, confirmed via `GET /v1/skills/perforce`)
— production's real corpus didn't happen to surface a term that cleanly
aliased into an existing skill during this validation window, which is a
fact about what real postings said this run, not a gap in the code path.

## What was built

**`internal/bedrock`**: a Bedrock Runtime client that batches up to 20
unmatched bullets per `InvokeModel` call (temperature 0, a strict system
prompt demanding a JSON array of `{bulletIndex, term, category, isRequired,
evidence, confidence}`), validates every response field-by-field — an
individual malformed finding is dropped, a structurally-invalid response
(not a JSON array at all) fails the whole call — and returns findings
attributed back to the bullet they came from by index.

**`internal/store`**: three new item families on the same table. A
`CACHE#<sha256>` bullet-result cache with a 90-day TTL (`GetBedrockCache`/
`PutBedrockCache`), storing only content-derived fields (term, category,
evidence, confidence) — deliberately *not* `isRequired`, since that's a
property of which posting section a bullet occurred in, not of the bullet
text itself, so it can't be safely shared across postings that happen to
reuse the same phrasing. A `REVIEW#PENDING`/`TERM#<normalized>` occurrence
counter with capped evidence accumulation (`RecordReviewOccurrence`,
`ListPendingReviews` — a real `Query` against the fixed `REVIEW#PENDING`
partition, not a `Scan`) plus a `REVIEW#REJECTED` extension beyond the
design's literal two-column table, so a rejected term doesn't just
reappear the next time a new posting reuses the same phrase. And a
`SKILL#<id>`/`META` canonical-skill writeback (`PutSkill`/`GetSkill`/
`ListDynamicSkills`) — see "DynamoDB instead of a git commit" below.

**`internal/extract`**: `FindUnmatchedBullets` re-scans the same
requirement/nice-to-have sections Stage 3 already looked at and returns
every line none of the compiled skills matched. `extract.Version` bumped
1 → 2, since every posting extracted before Phase 5 only ever saw
dictionary matches.

**`cmd/worker`**: restructured from one-message-at-a-time to batch
processing — Stages 1-3 run per message as before, but Stage 4 batches
unmatched bullets *across* the whole `ReceiveMessage` batch (up to 10
postings) before calling Bedrock, since this corpus's real rate of ~3
unmatched bullets/posting would barely batch at all per-message. A
Bedrock finding is checked against the *current* compiled dictionary
(`resolveFinding`, reusing Stage 3's own regex matchers via a newly
exported `CompiledSkill.FindEvidence`) before being treated as "unknown" —
a paraphrase Bedrock resolves to text the dictionary would also match
becomes a `method:"llm"` edge directly, not a spurious review-queue entry.
The worker also re-merges yaml-seeded + DynamoDB-approved skills every 5
minutes (`reloadSkills`) so a review approval takes effect without a
redeploy.

**`internal/api`**: `GET /v1/reviews` and `POST /v1/reviews/{term}`
(create/alias/reject). `Server.Skills`/`skillsByID` became
mutex-protected and refreshable (`RefreshSkills`) instead of a
load-once-at-startup snapshot; `cmd/api` runs the same periodic-reload
loop `cmd/worker` does.

**`cmd/rollup reextract`**: scans every active (non-closed) posting,
re-enqueues the ones whose stamped `ExtractVer` is behind
`extract.Version`, resending the existing `RawS3Key` rather than
refetching from the source ATS. The Bedrock cache means most of a re-run
costs nothing even though the dictionary changed.

**Terraform + Jenkins**: `service-worker`'s task role gets
`bedrock:InvokeModel` (see "the model that wasn't there" below) and
`dynamodb:Scan` (for `ListDynamicSkills`); `rollup_task` gets
`sqs:SendMessage` on the extract queue. A new manual `reextract` Jenkins
job runs `cmd/rollup reextract` via the existing
`job-syllabus-rollup-reconcile` task definition with a command override —
same pattern as `backfill`, no new build pipeline needed since
`rollup-build` already produces the image both subcommands run from.

## DynamoDB instead of a git commit

The design describes approved review terms being "written back to
`data/skills.yaml` via an API endpoint that opens a commit" — this project
has no GitHub write credential configured anywhere (Jenkins' own
`infra-plan`/`infra-apply` jobs are push-triggered rather than true
PR-discovery for the exact same reason, see `ci/jobs.groovy`'s comment),
so a literal git-commit-from-`cmd/api` was out of scope without adding a
new credential. Operator decision going in (confirmed before writing any
code): DynamoDB is the live source of truth instead. `data/skills.yaml`
stays the git-tracked seed loaded at startup; `config.MergeSkills` unions
it with whatever's in DynamoDB, DynamoDB winning on ID collision. This
satisfies the actual DoD ("approving one via the API updates the
dictionary and re-extraction picks it up") without inventing a credential
— but it does mean `data/skills.yaml` itself drifts out of sync with the
live dictionary until someone hand-copies approved entries back into it,
which is a real, accepted gap (see "What would be done differently").

## What broke

1. **The model the design named doesn't exist in this account's Bedrock
   catalog.** `anthropic.claude-3-5-haiku-20241022-v1:0` isn't in
   `aws bedrock list-foundation-models --region us-west-2` at all — it's
   been superseded. The available Haiku model
   (`anthropic.claude-haiku-4-5-20251001-v1:0`) also turned out to only
   support `INFERENCE_PROFILE` invocation, not on-demand
   (`aws bedrock get-foundation-model`'s `inferenceTypesSupported`), so
   `internal/bedrock.ModelID` had to become a cross-region inference
   profile ID (`us.anthropic.claude-haiku-4-5-20251001-v1:0`,
   `aws bedrock list-inference-profiles`), not a bare foundation-model ID
   — found and fixed *after* the first `terraform apply` had already
   granted `InvokeModel` on the wrong (nonexistent) resource ARN, caught
   by a direct `aws bedrock-runtime invoke-model` sanity check before
   trusting the IAM grant was even useful.
2. **Bedrock IAM for an inference profile needs two statements, not one.**
   `InvokeModel` against `us.anthropic.claude-haiku-4-5-...` alone isn't
   sufficient — Bedrock checks the calling principal against *both* the
   inference-profile ARN and whichever underlying regional foundation-model
   ARN actually serves the request (`aws bedrock get-inference-profile`
   lists all three: us-east-1, us-east-2, us-west-2). Missing the second
   statement would have produced a real `AccessDeniedException` only once
   Bedrock happened to route a request through one of those regions —
   caught by reading the profile's own routing list rather than guessing.
3. **`EXTRACT_QUEUE_URL`, added to the rollup-reconcile task-scheduled
   module's `environment` block, silently did nothing on `terraform
   apply`.** `modules/task-scheduled`'s task definition has
   `lifecycle.ignore_changes = [container_definitions]` (deliberately, so
   Jenkins-registered revisions aren't fought over) — meaning Terraform's
   own env-var addition is real for a *fresh* environment's bootstrap
   revision but completely inert against this already-deployed one. Fixed
   by making `rollup-build`'s existing "copy current task def, swap image"
   registration step also upsert `EXTRACT_QUEUE_URL` into
   `containerDefinitions[0].environment` every build, so Jenkins — which
   already owns the live revision in practice — explicitly owns this env
   var too, self-healing regardless of what's currently registered.
4. **Every real Bedrock call from the worker's production task role
   failed with `ResourceNotFoundException: Model use case details have
   not been submitted for this account.`** — a one-time, account-level
   requirement for first-time Anthropic model customers on Bedrock,
   submitted through a form in the console (Bedrock → Model access), not
   available via CLI/API at all. Confirmed via CloudWatch Logs
   (`worker: bedrock classify failed, skipping this chunk`) during the
   first real `reextract` run against production — 145 postings processed
   with dict-only results, zero crashes, zero DLQ entries, exactly the
   designed degradation. Operator submitted the form; access cleared
   (took longer than the error's own "try again in 15 minutes" estimate —
   closer to 30-40 minutes of real propagation before `InvokeModel`
   consistently succeeded from the worker's task role, despite `aws iam
   simulate-principal-policy`-equivalent direct CLI tests succeeding
   almost immediately from an operator session). This is specific to
   *this Anthropic model on Bedrock*, unrelated to and not fixable via
   any IAM change.
5. **`service-api`'s task role had no `dynamodb:PutItem`/`DeleteItem`** —
   Phase 3's own IAM comment had flagged this exact gap in advance
   ("Phase 3 is read-only... widen this when a write endpoint (submit/
   reviews) lands") but Phase 5 didn't act on the reminder until the
   first real `POST /v1/reviews/{term}` against production 500'd with
   `AccessDeniedException` on `PutItem`. Fixed by widening the existing
   managed policy (`modules/service-api/iam.tf`) to add `PutItem` +
   `DeleteItem` — an in-place policy-version update, not a new resource,
   so it sidesteps the `iam:TagPolicy` gap Phase 4 hit creating brand-new
   managed policies. Real IAM propagation for this change took
   noticeably longer than the "a few seconds, rarely a few minutes" AWS
   documents — closer to 5-10 minutes before `PutItem` consistently
   succeeded, even though `aws iam simulate-principal-policy` reported it
   as allowed well before real calls started succeeding. Two real
   findings in one debugging session with the same shape: **AWS's own
   policy simulator confirming "allowed" is not proof a live call will
   succeed yet** — propagation lag is real and can outlast both the
   error message's own estimate and the simulator's read of the current
   policy state.

## What would be done differently

- **`data/skills.yaml` drifting from DynamoDB's live dictionary is a real,
  ongoing gap, not a one-time cost.** Every approval widens the gap a
  little further. A follow-up worth doing before this matters: a small
  `cmd/rollup export-skills` (or similar) that dumps DynamoDB's canonical
  skills as yaml, diffed by hand against `data/skills.yaml` periodically —
  cheap to build, and turns "drift accumulates silently" into "drift is
  visible and reconcilable on demand."
- **Verify the model exists in the target account/region before writing
  code against its name from documentation.** The design doc's
  `claude-3-5-haiku` was accurate when written; account-side model
  catalogs change. `aws bedrock list-foundation-models` /
  `get-foundation-model` / `list-inference-profiles` cost nothing and
  would have caught both the missing model and the inference-profile
  requirement before any Go code assumed a bare on-demand model ID.
- **The `lifecycle.ignore_changes` gotcha (bug #3) is the same lesson as
  Phase 4's bug #9 (no ingest/rollup build pipeline) wearing a different
  hat**: a Terraform module block that looks like it configures something
  doesn't, once a resource inside it is explicitly excluded from Terraform's
  own reconciliation. Worth a standing habit: after adding any field to a
  `task-scheduled` module instantiation's `environment` list on an
  environment that's already been applied once, check whether that field
  needs a Jenkins-side change too, not just a `terraform apply`.
- **Bug #5's IAM gap was flagged in a code comment during Phase 3 and
  still got missed until a real 500 in production.** A comment saying
  "widen this when X lands" is a note to a future reader, not a
  mechanism that enforces anything — it's only as good as someone
  actually re-reading it at the right time. Nothing here would have
  caught it earlier short of actually exercising the write endpoint
  against a real deployment before calling the phase done, which is
  exactly what surfaced it. Worth generalizing: whenever a new
  write-capable API endpoint is added, explicitly check every IAM role
  in the request path for write permissions, don't rely on remembering a
  comment from two phases ago.
- **IAM/Bedrock propagation delays (bugs #4 and #5) were both slower than
  documented, and slower than what the policy simulator or a direct CLI
  test from a different principal suggested.** Budget real wall-clock
  time — tens of minutes, not seconds — after any IAM or account-level
  access change before concluding it hasn't taken effect, and don't trust
  `simulate-principal-policy` or a same-account-different-principal test
  as proof a *specific* role's *specific* calls will succeed yet.
