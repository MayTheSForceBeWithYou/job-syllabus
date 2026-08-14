# Bootstrap: the chicken-and-egg step

Every Terraform stack in this project (`envs/dev-data`, `envs/dev-compute`, and
later `envs/prod`) uses a remote S3 backend for state, with DynamoDB for
locking. Something has to create that S3 bucket and DynamoDB table before
any other stack can use them — and that something can't itself use a remote
backend it doesn't have yet.

`infra/terraform/bootstrap/` is that something. It uses a **local** backend
deliberately, is run once by hand, and creates:

- `job-syllabus-tfstate-<account_id>` — S3 bucket for all other stacks'
  state, versioned, SSE-S3 encrypted, all public access blocked.
  `prevent_destroy` guarded.
- `job-syllabus-tfstate-lock` — DynamoDB table for state locking
  (PAY_PER_REQUEST, effectively free at this scale). `prevent_destroy`
  guarded.
- A $60/month AWS Budget with email notifications at 80% actual, 100%
  actual, and 100% forecasted — set up *before* any other stack, per
  `docs/design.md` §12 ("Set an AWS Budget alarm at $60 before Phase 2, not
  after").

## Running it

```
cd infra/terraform/bootstrap
cp terraform.tfvars.example terraform.tfvars   # fill in your own alert_email
terraform init
terraform plan -out=bootstrap.tfplan
terraform apply "bootstrap.tfplan"
```

`terraform.tfvars` is gitignored (this is a public repo — don't commit an
email address into it). Only `terraform.tfvars.example` is tracked.

**AWS Budgets email notifications require a confirmation click** — check
your inbox after applying and confirm the subscription, or the alerts will
silently never fire.

## After it runs

Every other stack's `backend.tf` references the bucket/table by their fixed
names directly (`job-syllabus-tfstate-<account_id>` /
`job-syllabus-tfstate-lock`), using a distinct `key` per stack:

| Stack | State key |
|---|---|
| `envs/dev-data` | `dev-data/terraform.tfstate` |
| `envs/dev-compute` | `dev-compute/terraform.tfstate` |
| `envs/prod` (future) | `prod/terraform.tfstate` |

Re-running `bootstrap` after the first time is a no-op (nothing should have
drifted — it's not part of the normal apply/destroy cycle the rest of this
project uses for cost control).
