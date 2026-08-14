# Reusable EventBridge Scheduler -> ecs:RunTask module (docs/design.md §5
# "Scheduling", §14 repo layout) — one instantiation per scheduled job
# (cmd/ingest daily, cmd/rollup daily, cmd/rollup reconcile), rather than
# a bespoke resource block per job, since all three are the same shape:
# a one-shot Fargate task on a cron.

data "aws_ecs_cluster" "this" {
  cluster_name = var.ecs_cluster_name
}

resource "aws_ecs_task_definition" "this" {
  family                   = "${var.project}-${var.name}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.cpu
  memory                   = var.memory
  execution_role_arn       = var.task_execution_role_arn
  task_role_arn            = var.task_role_arn

  container_definitions = jsonencode([
    {
      name    = var.name
      image   = "${var.ecr_repo_url}:${var.image_tag}"
      command = var.command
      environment = [
        for e in var.environment : { name = e.name, value = e.value }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = var.log_group_name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = var.name
        }
      }
    }
  ])

  # Terraform only creates the initial revision, same bootstrap-placeholder
  # rationale as modules/service-api/service-worker — Jenkins registers
  # every real revision after the first image push.
  lifecycle {
    ignore_changes = [container_definitions]
  }

  tags = { Name = "${var.project}-${var.name}" }
}

# EventBridge Scheduler assumes this role to call RunTask on our behalf —
# distinct from both the task execution role (what the container itself
# can do) and the task role (what the running app can do).
data "aws_iam_policy_document" "scheduler_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["scheduler.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "scheduler" {
  name               = "${var.project}-${var.name}-scheduler"
  assume_role_policy = data.aws_iam_policy_document.scheduler_assume.json
}

data "aws_iam_policy_document" "scheduler_run_task" {
  statement {
    sid       = "RunTask"
    actions   = ["ecs:RunTask"]
    resources = [replace(aws_ecs_task_definition.this.arn, "/:\\d+$/", ":*")]
  }
  statement {
    sid     = "PassTaskRoles"
    actions = ["iam:PassRole"]
    resources = [
      var.task_execution_role_arn,
      var.task_role_arn,
    ]
  }
}

resource "aws_iam_role_policy" "scheduler_run_task" {
  name   = "${var.project}-${var.name}-run-task"
  role   = aws_iam_role.scheduler.id
  policy = data.aws_iam_policy_document.scheduler_run_task.json
}

resource "aws_scheduler_schedule" "this" {
  name       = "${var.project}-${var.name}"
  group_name = "default"

  flexible_time_window {
    mode = "OFF"
  }

  schedule_expression = var.schedule_expression

  target {
    arn      = data.aws_ecs_cluster.this.arn
    role_arn = aws_iam_role.scheduler.arn

    ecs_parameters {
      task_definition_arn = aws_ecs_task_definition.this.arn
      launch_type         = "FARGATE"

      network_configuration {
        subnets          = var.subnet_ids
        security_groups  = var.security_group_ids
        assign_public_ip = true # no NAT Gateway (docs/design.md §9 option 3)
      }
    }

    retry_policy {
      maximum_retry_attempts = 2
    }
  }
}
