# Task ROLE (what the running container can call), distinct from the
# shared task EXECUTION role (ECR pull + CloudWatch Logs, from
# modules/ecs-cluster) — least-privilege, scoped to exactly this table
# per docs/design.md §11.
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
  name               = "${var.project}-service-api-task"
  assume_role_policy = data.aws_iam_policy_document.task_assume.json
}

# Phase 3 is read-only (docs/design.md §7/§13) — no PutItem/UpdateItem
# here. Widen this when a write endpoint (submit/reviews) lands.
data "aws_iam_policy_document" "task_dynamodb_read" {
  statement {
    sid = "ReadJobSyllabusTable"
    actions = [
      "dynamodb:GetItem",
      "dynamodb:Query",
      "dynamodb:Scan",
      "dynamodb:BatchGetItem",
      "dynamodb:DescribeTable",
    ]
    resources = [
      var.table_arn,
      "${var.table_arn}/index/*",
    ]
  }
}

resource "aws_iam_policy" "task_dynamodb_read" {
  name   = "${var.project}-service-api-dynamodb-read"
  policy = data.aws_iam_policy_document.task_dynamodb_read.json
}

resource "aws_iam_role_policy_attachment" "task_dynamodb_read" {
  role       = aws_iam_role.task.name
  policy_arn = aws_iam_policy.task_dynamodb_read.arn
}
