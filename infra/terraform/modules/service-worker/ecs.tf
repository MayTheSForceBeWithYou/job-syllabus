resource "aws_ecs_task_definition" "worker" {
  family                   = "${var.project}-worker"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.cpu
  memory                   = var.memory
  execution_role_arn       = var.task_execution_role_arn
  task_role_arn            = aws_iam_role.task.arn

  # cmd/worker's Dockerfile stage (--build-arg BINARY=worker) copies data/
  # to /data/ in the final image, same convention service-api's task
  # definition already follows.
  container_definitions = jsonencode([
    {
      name  = "worker"
      image = "${var.ecr_repo_url}:${var.image_tag}"
      environment = [
        { name = "AWS_REGION", value = var.aws_region },
        { name = "SKILLS_FILE", value = "/data/skills.yaml" },
        { name = "RAW_BUCKET", value = split(":::", var.raw_bucket_arn)[1] },
        { name = "EXTRACT_QUEUE_URL", value = var.extract_queue_url },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = var.log_group_name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "worker"
        }
      }
    }
  ])

  # Same bootstrap-placeholder rationale as modules/service-api: Terraform
  # only creates the initial revision (possibly before any worker image
  # has ever been pushed); every real deploy after that comes from
  # Jenkins registering a new revision directly (describe -> swap image ->
  # register -> update-service), not from terraform apply.
  lifecycle {
    ignore_changes = [container_definitions]
  }

  tags = { Name = "${var.project}-worker" }
}

resource "aws_ecs_service" "worker" {
  name            = "worker"
  cluster         = var.ecs_cluster_id
  task_definition = aws_ecs_task_definition.worker.arn
  desired_count   = var.min_capacity
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.public_subnet_ids
    security_groups  = [var.fargate_sg_id]
    assign_public_ip = true # no NAT Gateway (docs/design.md §9 option 3) — needs a public IP to reach ECR/SQS/S3/DynamoDB
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  # Same rationale as modules/service-api: Jenkins owns which revision is
  # live after the first real deploy.
  lifecycle {
    ignore_changes = [task_definition, desired_count]
  }

  tags = { Name = "${var.project}-worker" }
}

resource "aws_appautoscaling_target" "worker" {
  min_capacity       = var.min_capacity
  max_capacity       = var.max_capacity
  resource_id        = "service/${var.ecs_cluster_name}/${aws_ecs_service.worker.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}

# Queue-depth-driven step scaling (docs/design.md §9 sizing table: "0-4
# tasks scaled on SQS ApproximateNumberOfMessagesVisible") — not
# target-tracking like service-api's CPU policy, because there's no
# meaningful "CPU utilization" signal for a service that's legitimately
# idle at 0 tasks most of the day between ingest runs. Two independent
# alarms instead: one scales out when the backlog appears, one scales
# back to zero once it's drained.
resource "aws_appautoscaling_policy" "worker_scale_out" {
  name               = "${var.project}-worker-scale-out"
  policy_type        = "StepScaling"
  resource_id        = aws_appautoscaling_target.worker.resource_id
  scalable_dimension = aws_appautoscaling_target.worker.scalable_dimension
  service_namespace  = aws_appautoscaling_target.worker.service_namespace

  step_scaling_policy_configuration {
    adjustment_type         = "ChangeInCapacity"
    cooldown                = 60
    metric_aggregation_type = "Maximum"
    step_adjustment {
      metric_interval_lower_bound = 0
      scaling_adjustment          = var.max_capacity
    }
  }
}

resource "aws_appautoscaling_policy" "worker_scale_in" {
  name               = "${var.project}-worker-scale-in"
  policy_type        = "StepScaling"
  resource_id        = aws_appautoscaling_target.worker.resource_id
  scalable_dimension = aws_appautoscaling_target.worker.scalable_dimension
  service_namespace  = aws_appautoscaling_target.worker.service_namespace

  step_scaling_policy_configuration {
    adjustment_type         = "ExactCapacity"
    cooldown                = 60
    metric_aggregation_type = "Maximum"
    step_adjustment {
      metric_interval_upper_bound = 0
      scaling_adjustment          = 0
    }
  }
}

resource "aws_cloudwatch_metric_alarm" "worker_queue_depth_high" {
  alarm_name          = "${var.project}-worker-queue-depth-high"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 1
  metric_name         = "ApproximateNumberOfMessagesVisible"
  namespace           = "AWS/SQS"
  period              = 60
  statistic           = "Maximum"
  threshold           = var.scale_out_queue_depth
  dimensions = {
    QueueName = element(split("/", var.extract_queue_url), length(split("/", var.extract_queue_url)) - 1)
  }
  alarm_actions = [aws_appautoscaling_policy.worker_scale_out.arn]
  tags          = { Name = "${var.project}-worker-queue-depth-high" }
}

resource "aws_cloudwatch_metric_alarm" "worker_queue_depth_zero" {
  alarm_name          = "${var.project}-worker-queue-depth-zero"
  comparison_operator = "LessThanOrEqualToThreshold"
  evaluation_periods  = var.scale_in_evaluation_periods
  metric_name         = "ApproximateNumberOfMessagesVisible"
  namespace           = "AWS/SQS"
  period              = 60
  statistic           = "Maximum"
  threshold           = 0
  dimensions = {
    QueueName = element(split("/", var.extract_queue_url), length(split("/", var.extract_queue_url)) - 1)
  }
  alarm_actions = [aws_appautoscaling_policy.worker_scale_in.arn]
  tags          = { Name = "${var.project}-worker-queue-depth-zero" }
}
