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

# GetItem (GetPosting) + PutItem (PutPosting, and the edge half of the
# PutSkillEdge transaction) + TransactWriteItems (PutSkillEdge itself,
# docs/design.md §4) — no Query/Scan, cmd/worker never lists anything, it
# only ever looks up postings it was handed a specific ID for.
data "aws_iam_policy_document" "task_dynamodb" {
  statement {
    sid = "ReadWriteJobSyllabusTable"
    actions = [
      "dynamodb:GetItem",
      "dynamodb:PutItem",
      "dynamodb:TransactWriteItems",
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
