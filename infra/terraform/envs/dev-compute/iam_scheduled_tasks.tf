# Task roles for the two EventBridge-scheduled tasks (modules/task-scheduled
# instantiations below) — kept here rather than inside that module since,
# unlike modules/service-worker's single binary, cmd/ingest and cmd/rollup
# need genuinely different permissions and each only ever backs one
# scheduled job, so there's no shared module to own them.

data "aws_iam_policy_document" "scheduled_task_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

# cmd/ingest (docs/design.md §9): fetches + dedupes + upserts postings
# (DynamoDB read/write), uploads raw content (S3 write under raw/), and
# enqueues extraction work (SQS send). No TransactWriteItems — only
# cmd/worker writes skill edges/counters.
resource "aws_iam_role" "ingest_task" {
  name               = "job-syllabus-ingest-task"
  assume_role_policy = data.aws_iam_policy_document.scheduled_task_assume.json
}

data "aws_iam_policy_document" "ingest_task" {
  statement {
    sid = "ReadWriteJobSyllabusTable"
    actions = [
      "dynamodb:GetItem",
      "dynamodb:PutItem",
      "dynamodb:DeleteItem",
      "dynamodb:Query",
      "dynamodb:DescribeTable",
      "dynamodb:UpdateTimeToLive",
    ]
    resources = [
      data.terraform_remote_state.data.outputs.table_arn,
      "${data.terraform_remote_state.data.outputs.table_arn}/index/*",
    ]
  }
  statement {
    sid       = "WriteRawPostingContent"
    actions   = ["s3:PutObject"]
    resources = ["${data.terraform_remote_state.data.outputs.data_bucket_arn}/raw/*"]
  }
  statement {
    sid       = "EnqueueExtraction"
    actions   = ["sqs:SendMessage"]
    resources = [module.queues.queue_arns["extract"]]
  }
}

resource "aws_iam_role_policy" "ingest_task" {
  name   = "job-syllabus-ingest-task"
  role   = aws_iam_role.ingest_task.id
  policy = data.aws_iam_policy_document.ingest_task.json
}

# cmd/rollup (docs/design.md §4/§9): reconcile scans+corrects counters
# (DynamoDB scan/read/write); export/import (manual, the dev-data teardown
# safety net) read/write the whole table plus the backups/ S3 prefix;
# reextract (Phase 5) scans postings and enqueues the ones behind current
# ExtractVer (SQS send). One role covers the whole binary rather than
# juggling per-subcommand roles, since the scheduled job and the manual
# invocations share the same image.
resource "aws_iam_role" "rollup_task" {
  name               = "job-syllabus-rollup-task"
  assume_role_policy = data.aws_iam_policy_document.scheduled_task_assume.json
}

data "aws_iam_policy_document" "rollup_task" {
  statement {
    sid = "ReadWriteJobSyllabusTable"
    actions = [
      "dynamodb:GetItem",
      "dynamodb:PutItem",
      "dynamodb:BatchWriteItem",
      "dynamodb:Scan",
      "dynamodb:DescribeTable",
    ]
    resources = [
      data.terraform_remote_state.data.outputs.table_arn,
      "${data.terraform_remote_state.data.outputs.table_arn}/index/*",
    ]
  }
  statement {
    sid = "BackupBucketAccess"
    actions = [
      "s3:GetObject",
      "s3:PutObject",
    ]
    resources = ["${data.terraform_remote_state.data.outputs.data_bucket_arn}/backups/*"]
  }
  statement {
    sid       = "EnqueueReextraction"
    actions   = ["sqs:SendMessage"]
    resources = [module.queues.queue_arns["extract"]]
  }
}

resource "aws_iam_role_policy" "rollup_task" {
  name   = "job-syllabus-rollup-task"
  role   = aws_iam_role.rollup_task.id
  policy = data.aws_iam_policy_document.rollup_task.json
}
