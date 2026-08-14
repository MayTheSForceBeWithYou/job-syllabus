# Phase 3 — Deploy API

**Status: done. DoD passes.** `curl`ing the real API Gateway URL returns
ranked skills computed from real ingested data (71 postings, 5 companies),
deployed by Jenkins from a git push — no manual `terraform apply` or manual
container push involved in the final validated path (`docs/design.md` §13's
DoD).

```
$ curl https://o79zaeqqna.execute-api.us-west-1.amazonaws.com/prod/v1/skills?limit=5
{"skills":[
  {"id":"aws","display":"Amazon Web Services (AWS)","category":"cloud","count":16,...,"pctOfPostings":22.5},
  {"id":"kubernetes","display":"Kubernetes","category":"containers","count":16,...,"pctOfPostings":22.5},
  {"id":"gcp","display":"Google Cloud Platform (GCP)","category":"cloud","count":14,...,"pctOfPostings":19.7},
  ...
],"totalPostings":71}
```

## What was built

**Read-only REST API** (`internal/api/`): chi router behind `/healthz`,
`/readyz` (unauthenticated) and `/v1/*` (behind an IP-allowlist
middleware — see below). Endpoints: `/v1/skills` (with
roleFamily/tier/required/since/limit filters), `/v1/skills/{id}`,
`/v1/skills/{id}/postings`, `/v1/postings`, `/v1/postings/{id}`,
`/v1/companies`, `/v1/stats/overview`. RFC 7807 problem+json errors,
cursor-based pagination (`internal/store/cursor.go`, base64 of DynamoDB's
`LastEvaluatedKey`), request-ID echoing. Ranking logic
(`internal/rank/rank.go`) was extracted out of `cmd/ingest`'s report
command so both the CLI and the API compute identical numbers from the
same code.

**ECS Fargate + internal ALB + API Gateway** (`infra/terraform/modules/
service-api/`, `modules/api-gateway/`): `service-api` runs behind an
internal (no public IP) ALB; an HTTP API (v2) reaches it through a VPC
Link. The task definition and service both `lifecycle { ignore_changes }`
their container/task-definition fields after the first apply — Jenkins
owns every deploy after that, not Terraform.

**No WAF — application-layer IP allowlist instead**
(`internal/api/ipallow.go`). AWS WAFv2's `AssociateWebACL` does not
support HTTP APIs as a resource type at all (only REST APIs, confirmed
against a real failing apply and AWS's own API reference), so Phase 3's
"no auth yet, locked to your IP" requirement is enforced in the app
instead, reading a client IP header set by an API Gateway integration
parameter mapping (see "What broke" — getting the *right* header took two
attempts).

**Jenkins CI/CD** (`ci/Jenkinsfile.api-build`): vet → lint → test →
extraction-precision gate → build (Kaniko, on a dedicated minimal agent) →
verify-in-ECR → Trivy (fail on HIGH+) → deploy (swap the task definition's
image, register a new revision, `update-service`, wait for stability) →
smoke test `/healthz` through the real API Gateway URL. `ci/
Jenkinsfile.infra-plan`/`infra-apply` got their first-ever real executions
this phase and needed real fixes to actually work (see below).

**Data**: the real AWS DynamoDB table had never been populated (`cmd/
ingest` only supports DynamoDB Local — see below). Populated via `cmd/
rollup export` (local DynamoDB → the real S3 data bucket) followed by
`cmd/rollup import` (S3 → the real table), the same backup/restore pair
built in Phase 2 for the dev-data teardown safety net, repurposed here as
the actual local-to-cloud data sync mechanism.

## What broke

This phase's CI/CD pipeline (`ci/jenkins.yaml`, `ci/Jenkinsfile.*`) was
*written* in Phase 2 but never actually *run* until this phase — every
bug below is something that had been sitting latent since Phase 2 and
only surfaced once a real build was pushed through it. In encountered
order:

1. **WAF can't front an HTTP API at all.** `AssociateWebACL` rejected the
   API Gateway stage's ARN outright ("The ARN isn't valid") regardless of
   percent-encoding or switching from `$default` to a named stage.
   WebFetching AWS's own `AssociateWebACL` reference confirmed HTTP APIs
   (protocol_type=HTTP) simply aren't a supported resource type — only
   REST APIs (v1) are. Pivoted to the application-layer `ipAllowlist`
   middleware instead of switching to a REST API (which would have
   deviated from `docs/design.md` §3's stated HTTP-API-with-JWT-authorizer
   design for Phase 6).
2. **Jenkins agents could never get an executor.** `RunTask` failed with
   `InvalidParameterException: Override argument cannot be null` — traced
   via the `amazon-ecs-plugin`'s own source to two missing JCasC fields:
   `agentContainerName` (needed to name the container-override target) and
   the ECS cloud's own `jenkinsUrl` (distinct from the global
   `unclassified.location.url`, needed to build the JNLP connect-back
   command). Fixing both wasn't enough on its own — see #3.
3. **Agents that did launch couldn't reach Jenkins.** The public ALB's
   security group only allows the operator's IP; Fargate agents have
   ephemeral public IPs from AWS's general pool, impossible to allowlist,
   and (confirmed via `nslookup`/`curl` from inside the VPC) the public
   ALB's DNS name resolves to public IPs even for same-VPC clients — no
   automatic private hairpin. Fixed by pinning the Jenkins instance's
   `private_ip` and pointing the ECS cloud's `jenkinsUrl` at it directly,
   with a same-VPC security-group rule instead of relying on the ALB. A
   further fix was needed even after that: the initial HTTP handshake
   only negotiates a *separate*, otherwise-random TCP port for the actual
   JNLP4-connect protocol — pinned via `slaveAgentPort: 50000`.
4. **The agent image didn't exist yet** (a chicken-and-egg: Jenkins can't
   build its own agent image without an agent). Built and pushed
   `ci/agent.Dockerfile` by hand, hitting two real build bugs along the
   way: `tfsec`'s install script needs `bash`, not `sh` (bash-only `+=`
   syntax silently broke under dash); `pip3 install` needed
   `--ignore-installed` (the apt-managed `packaging` package has no
   RECORD file for pip to check against).
5. **No Docker daemon on Fargate.** `docker buildx build` failed outright
   reaching `unix:///var/run/docker.sock` — no privileged DinD available,
   exactly as anticipated in `docs/design.md` §10 but never actually
   wired up. Switched the Build image stage to Kaniko (daemonless,
   userspace image builds).
6. **The app's own root `Dockerfile` had never been built as a real
   container before this phase.** It was still pinned to `golang:1.23`
   while `go.mod` had moved to `go 1.26.5` — "go.mod requires go >= 1.26.5
   (running go 1.23.12)." A one-line version bump, but a reminder that an
   unexercised Dockerfile is not a working Dockerfile.
7. **Kaniko sharing a container with the live JNLP process repeatedly
   killed the agent mid-build.** Kaniko has no daemon/overlayfs isolation
   — it unpacks the target image's base rootfs directly onto whatever
   container it's running in. On the general-purpose, tool-laden
   `fargate-agent` image, this corrupted paths the live JNLP process
   needed, killing the connection at the exact same point on every
   attempt regardless of CPU/memory sizing. Fixed by splitting Kaniko onto
   its own dedicated, minimal `kaniko-agent` (its own Dockerfile, ECS
   cloud template, and ECR repo — the latter needed a mutable tag policy
   since it's a `:latest`-tagged tooling image, unlike the immutable,
   git-SHA-tagged app images).
8. **Even isolated, Kaniko's between-stage filesystem reset still broke
   the agent.** Kaniko deletes the entire filesystem between Dockerfile
   stages (build → distroless) except explicitly protected paths, and
   `/home/jenkins` (the JNLP agent's own workspace and remoting jar cache)
   wasn't protected — the build got through the whole compile step and
   into "Deleting filesystem..." before the connection died. Fixed with
   `--ignore-path=/home/jenkins`.
9. **That still wasn't the whole story.** A further real run got all the
   way through the build *and* the push (a real image digest landed in
   ECR) but Jenkins still marked the build FAILURE ~8 minutes later:
   "wrapper script does not seem to be touching the log file." Root cause:
   the Durable Task Plugin's own post-completion bookkeeping needs
   external binaries (`mv`, `sh`) under `/bin`, `/usr`, `/lib`, `/etc` —
   all wiped by the same filesystem reset. The obvious fix (protect those
   paths too) broke something else: a further real run showed
   `--ignore-path` doesn't just protect a path from deletion, it excludes
   it from Kaniko's unpack step too — ignoring `/usr` meant the build
   stage's own `golang:1.26-bookworm` toolchain under `/usr/local/go`
   never got unpacked at all ("go: not found"). Kaniko's filesystem model
   and a long-lived JNLP agent process are fundamentally in tension for a
   multi-stage build, and no combination of `--ignore-path` resolves it
   cleanly. The actual fix: stop trusting this stage's exit status
   (`returnStatus: true` instead of failing hard) and add a dedicated
   stage that verifies the push independently against ECR — the real
   source of truth, unaffected by whatever happens to the agent locally
   afterward.
10. **A concurrent `pollSCM`-triggered build collided on an immutable ECR
    tag.** One real run's Kaniko push failed with `TAG_INVALID: ... tag is
    immutable` because an earlier, automatically-triggered build for the
    same commit had already pushed it. The ECR-verification stage from
    #9 absorbed this correctly too — it found the image already present
    (pushed by the other build) and treated it as success, which is
    actually the correct outcome.
11. **API Gateway forwards the stage prefix into the app's path.**
    `/prod/healthz` reached the app as literally `/prod/healthz`, which
    chi correctly 404'd (it only knows `/healthz`) — confirmed by
    inspecting the app's own RFC 7807 response body, not just the status
    code. Fixed with an `overwrite:path` request-parameter mapping on the
    integration, rewriting the outgoing path to just the captured
    `{proxy}` segment.
12. **`X-Forwarded-For` does not carry the real client IP through a
    private VPC Link integration.** Even from the operator's own allowed
    IP, `/v1/skills` 403'd. Added temporary server-side logging of the
    rejected request's headers and found the actual value: a private VPC
    address (`10.20.10.106`) — the VPC Link's own internal hop, not
    anything traceable to the original caller. Unlike ALB/CloudFront,
    API Gateway HTTP APIs' private integrations don't propagate the true
    client IP into `X-Forwarded-For` this way. Fixed by having the
    integration inject `$context.identity.sourceIp` (API Gateway's own
    authoritative view of the caller) into a dedicated `X-Real-Ip` header
    via `overwrite:header` parameter mapping — `overwrite:`, not
    `append:`, so a client can't supply their own value and have it
    survive.
13. **`infra-apply`'s actual first-ever run failed outright on missing
    variables.** `terraform.tfvars` is (correctly) gitignored — real
    operator IP/email have no business in a public repo — which also
    means it never reaches a Jenkins checkout. Fixed by having both
    `infra-plan` and `infra-apply` generate `terraform.tfvars.json` from
    SSM parameters before planning, the same pattern already used for the
    Jenkins admin password and the API URL.
14. **`infra-apply`'s plan stage passed, then failed on `archiveArtifacts`
    anyway.** The artifact glob still carried the full
    `infra/terraform/envs/dev-compute/` prefix even though it's evaluated
    relative to the enclosing `dir()` block, which had already changed
    into that directory — a doubled path that never matched anything.
    Never caught before because this job had never actually run.
15. **The real AWS DynamoDB table was simply empty.** `cmd/ingest` only
    ever supported DynamoDB Local (`internal/store.New`'s fake static
    credentials) — there is no "ingest against real AWS" mode. Populated
    the real table via the Phase 2 `cmd/rollup export`/`import` pair
    instead (local DynamoDB → S3 → real DynamoDB), which is what it was
    already built for.

## What would be done differently

- **A CI pipeline that's written but never run is not a working
  pipeline.** Every bug above except #15 was latent in code that had
  already been reviewed and merged in Phase 2 — none of it was caught
  until a real build actually went through the real infrastructure this
  phase. `ci/Jenkinsfile.*` and `ci/jenkins.yaml` should have been
  smoke-tested against a scratch pipeline the moment they were written,
  not treated as done because `terraform apply` succeeded and the YAML
  was well-commented.
- **Kaniko inside a long-lived JNLP agent process is fragile in a way
  that `--ignore-path` cannot fully paper over** (#7-9). The
  `returnStatus` + ECR-verification pattern is a reasonable pragmatic fix
  given the amazon-ecs-plugin's constraints, but a cleaner architecture —
  a genuinely ephemeral, single-purpose build step decoupled from the
  JNLP agent's own lifecycle (e.g. a sidecar container, or a separate
  build service entirely) — would avoid the whole class of problem rather
  than working around its symptom.
- **Don't trust a "how proxies conventionally behave" assumption about a
  specific AWS integration type without checking the actual header
  value.** The original `X-Forwarded-For`-trusting comment in
  `ipallow.go` was written by analogy to ALB/CloudFront and was simply
  wrong for API Gateway HTTP APIs' *private* VPC Link integrations —
  caught only by logging the real rejected value and reading it, not by
  re-reasoning about it harder.
- **Gitignored `terraform.tfvars` versus a Jenkins-executed apply is an
  obvious gap in hindsight** (#13) — SSM-as-source-of-apply-specific-values
  was already the established pattern (admin password, API URL) and
  should have been applied to the plan/apply Jenkinsfiles from the start,
  not discovered on the first real run of a job that had existed since
  Phase 2.
- **`cmd/ingest` having no real-AWS mode is a real gap worth closing**,
  not just working around via `cmd/rollup`. The export/import round-trip
  works but is a manual step; a later phase's scheduled/queued ingestion
  (the `worker`/`scraper`/queues machinery already scaffolded but not yet
  wired up) is the actual intended automation for this.
