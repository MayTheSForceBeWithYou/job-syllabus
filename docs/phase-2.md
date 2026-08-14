# Phase 2 — Terraform baseline + Jenkins

**Status: done. DoD passes.** The Jenkins EC2 instance was terminated,
`terraform apply` was run with no other changes, and Jenkins came back
fully configured — JCasC loaded clean, all three plugins-driven and
Job-DSL-seeded jobs (`api-build`, `infra-plan`, `infra-apply`) present, ALB
target healthy, `/login` returning HTTP 200 — with zero manual steps
(`docs/design.md` §9/§13's DoD). See [bootstrap.md](bootstrap.md) for the
one piece of infrastructure that predates everything else in this phase.

## What was built

**Terraform, split into three independent stacks** (`infra/terraform/`),
per an operator decision partway through this phase (`docs/design.md` §9,
"Amended 2026-08-13"): `bootstrap` (state bucket, lock table, $60 budget
alarm — run once, by hand); `envs/dev-data` (DynamoDB table, S3 data
bucket, ECR repos, Route53 zone — long-lived, `prevent_destroy` guarded,
near-free at rest); `envs/dev-compute` (network, ECS cluster, SQS queues,
observability, Jenkins — the expensive stuff, meant to be destroyed and
reapplied at will for cost control). `dev-compute` reads `dev-data`'s
outputs via `terraform_remote_state` rather than duplicating them.

**Data-loss safety net for the destroy/reapply workflow**: `cmd/rollup`
(`export`/`import` subcommands) dumps the whole DynamoDB table to a gzipped
JSON-lines file in S3 and restores from it — exercised end-to-end against
real AWS (280 items out, table wiped, 280 items back, `make report` output
identical) before being trusted as the answer to "what if `dev-data` itself
ever needs to come down."

**Jenkins** (`infra/terraform/modules/jenkins/`): t4g.small EC2 behind an
ALB, TLS via an ACM cert DNS-validated against a delegated subdomain
(`job-syllabus.skopekreep.com`, Route53-hosted, NS-delegated from the
operator's existing Squarespace-hosted apex domain — chosen specifically so
the Squarespace home page didn't need to move). `JENKINS_HOME` lives on a
separate EBS volume with a daily DLM snapshot policy (7-day retention) so
it can outlive an instance replacement. Zero static AWS credentials
anywhere: an IAM instance profile (scoped IAM-management policy, not full
admin) does everything Jenkins needs, including managing its own future
Terraform applies. The admin password is Terraform-generated
(`random_password`), stored as an SSM SecureString, and fetched at boot via
the instance role — never written to the (public) repo. Configuration is
entirely code: JCasC (`ci/jenkins.yaml`) defines the security realm, a
Fargate ECS cloud for ephemeral build agents, and a Job DSL seed
(`ci/jobs.groovy`) that creates the three pipeline jobs from
`ci/Jenkinsfile.*`. `ci/plugins.txt` + `jenkins-plugin-manager` install the
plugin set before Jenkins ever starts. Nothing here is ever configured
through the UI — the whole point of the DoD test is that UI changes
wouldn't survive a rebuild anyway.

**Supporting modules**: `network` (VPC sized to avoid the account's default
VPC, public-subnet-only Fargate per §9's no-NAT dev option, free S3/DynamoDB
gateway endpoints), `ecs-cluster` (FARGATE + FARGATE_SPOT), `queues`
(ingest/extract SQS + DLQs), `observability` (SNS alerts, DLQ-depth
alarms), `dns` (the Route53 zone, deliberately in `dev-data` so the manual
Squarespace delegation step is never repeated). `ci/agent.Dockerfile`
builds the ephemeral Fargate build-agent image (Go, Docker CLI+buildx,
Terraform, Trivy/tfsec/checkov, eas-cli).

## What broke

Nine real bugs and one live troubleshooting session, in the order
encountered — every one found by testing against real AWS behavior rather
than trusting a clean `terraform apply` exit code:

1. **DynamoDB table name mismatch.** `modules/data` named the table
   `var.project` ("job-syllabus"), but `internal/store.TableName` has been
   hardcoded `"jobsyllabus"` (no hyphen) since Phase 0/1. Fixed by renaming
   the Terraform resource to match the already-tested application
   convention, not the other way around.
2. **DLM snapshot policy `description` rejected by `terraform validate`.**
   AWS DLM's description field only allows a narrow character set — no
   `§`, and (on a second attempt) no commas or parens either.
3. **A backgrounded `apply | tail -100` reported the wrong exit code** —
   the task notification's "exit 0" was `tail`'s exit code, not
   `terraform apply`'s, because the pipe swallowed the real one. Fixed by
   using `set -o pipefail` on every subsequent apply.
4. **EC2 security group description with an apostrophe rejected by AWS**
   ("the operator's IP" → `InvalidParameterValue`) — AWS SG descriptions
   don't allow apostrophes, unlike Route53 zone comments, which do (found
   by grepping the whole tree for the same mistake after fixing the first
   instance).
5. **ACM certificate validation took ~32 minutes**, not a bug but a real
   delay: the Squarespace NS delegation for the subdomain hadn't propagated
   yet. Diagnosed live via `nslookup -type=NS`, then walked the operator
   through Squarespace's UI, which requires one NS record per nameserver
   value rather than a single multi-value record — the operator's first
   attempt had a nameserver value in the wrong form field.
6. **Nitro/NVMe device-symlink bug**: `/dev/xvdf` is a udev symlink to the
   real `/dev/nvmeXn1` device on Nitro instances (t4g included). `file -s`
   without `-L` describes the symlink itself, not its target, so a
   genuinely blank volume was never detected as blank, `mkfs` was skipped,
   and `mount` failed. Diagnosed via SSM `send-command` pulling
   `/var/log/cloud-init-output.log`. Fixed with `file -sL`.
7. **Jenkins' RPM repo moved out from under a hand-copied config.** The
   original `user_data.sh.tftpl` pointed at `redhat-stable` with a
   standalone `jenkins.io-2023.key`, which failed `GPG check FAILED` even
   after adding the missing `gpgkey=` line the first fix attempt was
   missing. Root cause, confirmed by fetching Jenkins' own current
   `https://pkg.jenkins.io/rpm-stable/jenkins.repo` live: the project has
   since moved to a `rpm-stable` path with repo-metadata signing
   (`repodata/repomd.xml.key` + `repo_gpgcheck=1`) instead of a standalone
   package key. A hand-transcribed config for a fast-moving upstream repo
   is a liability; fetching the canonical file directly and using it
   verbatim would have avoided this from the start.
8. **JCasC boot failure: `UnknownAttributesException`.** `ci/jenkins.yaml`
   used `executionRoleArn`/`taskRoleArn` for the Fargate agent template,
   but the installed `amazon-ecs` plugin's actual `ECSTaskTemplate` schema
   uses `executionRole`/`taskrole` — no `Arn` suffix. This was flagged as a
   risk ("best-effort, may need schema adjustment") when the plugin's
   config was first written from documentation rather than the plugin's
   own live schema; the risk materialized exactly as predicted. Fixed by
   reading the plugin's own "Available attributes" error output rather
   than trusting docs a second time.
9. **EC2 Instance Connect had no path in**: the `jenkins_ec2` security
   group only allowed port 8080 from the ALB, so the AWS Console's
   browser-SSH failed with "Port 22 not authorized." Fixed with an ingress
   rule scoped to the AWS-owned per-region Instance Connect service range
   (`13.52.6.112/29` for `us-west-1`), not the open internet.

One local (non-infrastructure) issue: an early SSM-polling loop rendered
`aws` CLI output straight to a Windows console still on a non-UTF-8
codepage, producing a wall of `'charmap' codec can't encode character`
errors that looked like remote failures but were purely a local rendering
bug. Setting `PYTHONUTF8=1`/`PYTHONIOENCODING=utf-8` before shelling out to
`aws` fixed it — worth defaulting to on this machine going forward.

## What would be done differently

- **Bugs 6-8 (the ones that actually blocked Jenkins from booting) only
  showed up on real EC2, and each one cost a full instance-replacement
  cycle to diagnose** (`terraform apply` → wait for boot → SSM
  `send-command` → read the log → fix → repeat). A faster inner loop —
  e.g. testing `user_data.sh` against a scratch instance directly with
  `aws ec2 run-instances` before wiring it into the real Terraform module —
  would have caught the device-symlink and GPG bugs without touching the
  "real" Jenkins instance three times.
- **Hand-transcribing a third party's config (bug 7) is exactly the kind
  of thing that silently rots.** Fetching the canonical file live (as the
  eventual fix did) should have been the first approach, not the fallback
  after a failure.
- **A plugin's documented config schema and its actual installed schema
  can diverge** (bug 8) — this was flagged as a risk in advance and still
  cost a full boot cycle to hit. Worth validating third-party JCasC
  snippets against the plugin's own schema-introspection output before the
  first real boot, not just before the first *manual* review.
- **SSM `send-command`/`get-command-invocation` polling, not
  `terraform apply`'s exit code, was what actually caught every one of
  bugs 6-9.** This matches the pattern from Phase 1 (real data over
  synthetic fixtures) applied to infrastructure: a clean `apply` proves
  Terraform did what it was told, not that the thing it built actually
  works. That should stay the default verification step for any
  boot-time infrastructure change, not a debugging technique reached for
  only after something looks wrong.
