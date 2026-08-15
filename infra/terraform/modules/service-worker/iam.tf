# Task ROLE (what the running container can call), distinct from the
# shared task EXECUTION role (ECR pull + CloudWatch Logs, from
# modules/ecs-cluster) — least-privilege, scoped to exactly what
# cmd/worker's own code paths use (docs/design.md §11).
#
# Inline (aws_iam_role_policy), not managed policies + attachment: a real
# apply confirmed the Jenkins CI role can create/tag IAM roles and inline
# role policies (both succeeded) but not standalone managed policies —
# CreatePolicy's implicit tagging needs iam:TagPolicy, which the CI role's
# scoped PowerUserAccess-based permissions don't grant. Inline policies
# don't hit that codepath at all, so this sidesteps the gap entirely
# rather than widening what the CI role can do to IAM.
data "aws_iam_policy_document" "task_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "task" {
  name               = "${var.project}-service-worker-task"
  assume_role_policy = data.aws_iam_policy_document.task_assume.json
}

# GetItem (GetPosting, Bedrock cache reads, rejected-term checks) + PutItem
# (PutPosting, the edge half of the PutSkillEdge transaction, Bedrock cache
# writes) + UpdateItem (the STAT# counter's ADD, the other half of that same
# transaction — IAM evaluates TransactWriteItems per-item against the
# single-item action matching each TransactItem's own operation, not just
# against TransactWriteItems itself; a real run confirmed this the hard way
# with AccessDeniedException on UpdateItem specifically, PutItem alone
# wasn't enough — plus the review-queue occurrence counter's own ADD) +
# TransactWriteItems (docs/design.md §4) + Scan (Phase 5:
# ListDynamicSkills, reading DynamoDB-approved skills for the periodic
# dictionary merge — same accepted Phase-1-scale tradeoff as every other
# Scan in this codebase) — no Query, cmd/worker never queries a GSI.
data "aws_iam_policy_document" "task_dynamodb" {
  statement {
    sid = "ReadWriteJobSyllabusTable"
    actions = [
      "dynamodb:GetItem",
      "dynamodb:PutItem",
      "dynamodb:UpdateItem",
      "dynamodb:TransactWriteItems",
      "dynamodb:Scan",
    ]
    resources = [var.table_arn]
  }
}

resource "aws_iam_role_policy" "task_dynamodb" {
  name   = "${var.project}-service-worker-dynamodb"
  role   = aws_iam_role.task.id
  policy = data.aws_iam_policy_document.task_dynamodb.json
}

# rawstore.Get only ever reads (GetObject) — cmd/ingest, not cmd/worker,
# is what writes raw content to S3.
data "aws_iam_policy_document" "task_s3_read" {
  statement {
    sid       = "ReadRawPostingContent"
    actions   = ["s3:GetObject"]
    resources = ["${var.raw_bucket_arn}/raw/*"]
  }
}

resource "aws_iam_role_policy" "task_s3_read" {
  name   = "${var.project}-service-worker-s3-read"
  role   = aws_iam_role.task.id
  policy = data.aws_iam_policy_document.task_s3_read.json
}

data "aws_iam_policy_document" "task_sqs" {
  statement {
    sid = "ConsumeExtractQueue"
    actions = [
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:GetQueueAttributes",
    ]
    resources = [var.extract_queue_arn]
  }
}

resource "aws_iam_role_policy" "task_sqs" {
  name   = "${var.project}-service-worker-sqs"
  role   = aws_iam_role.task.id
  policy = data.aws_iam_policy_document.task_sqs.json
}

data "aws_caller_identity" "current" {}

# Stage 4 (docs/design.md §6) — Bedrock has no us-west-1 presence, so this
# grants InvokeModel in us-west-2 (internal/bedrock.Region), scoped to
# specific model ARNs, not a wildcard resource. Two resource types, both
# required: the inference-profile ARN itself (account-scoped — this is what
# ModelID actually names in the InvokeModel call) AND every underlying
# foundation-model ARN the profile can route a request to across its three
# backing regions (`aws bedrock get-inference-profile` — Bedrock IAM checks
# both the profile and whichever regional foundation model actually serves
# the request). Scoped to on-demand invocation only; no batch/provisioned-
# throughput actions, since cmd/worker only ever does synchronous
# per-batch InvokeModel calls.
data "aws_iam_policy_document" "task_bedrock" {
  statement {
    sid     = "InvokeHaikuInferenceProfile"
    actions = ["bedrock:InvokeModel"]
    resources = [
      "arn:aws:bedrock:${var.bedrock_region}:${data.aws_caller_identity.current.account_id}:inference-profile/${var.bedrock_inference_profile_id}",
    ]
  }
  statement {
    sid     = "InvokeHaikuUnderlyingFoundationModel"
    actions = ["bedrock:InvokeModel"]
    resources = [
      for region in var.bedrock_underlying_regions :
      "arn:aws:bedrock:${region}::foundation-model/${var.bedrock_foundation_model_id}"
    ]
  }
}

resource "aws_iam_role_policy" "task_bedrock" {
  name   = "${var.project}-service-worker-bedrock"
  role   = aws_iam_role.task.id
  policy = data.aws_iam_policy_document.task_bedrock.json
}
