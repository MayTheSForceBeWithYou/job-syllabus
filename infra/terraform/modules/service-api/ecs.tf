resource "aws_ecs_task_definition" "api" {
  family                   = "${var.project}-api"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.cpu
  memory                   = var.memory
  execution_role_arn       = var.task_execution_role_arn
  task_role_arn            = aws_iam_role.task.arn

  # cmd/api's Dockerfile stage (--build-arg BINARY=api) copies data/ to
  # /data/ in the final image, not ./data/ — SKILLS_FILE/COMPANIES_FILE
  # must point there, unlike cmd/ingest's local defaults.
  container_definitions = jsonencode([
    {
      name  = "api"
      image = "${var.ecr_repo_url}:${var.image_tag}"
      portMappings = [
        { containerPort = 8080, protocol = "tcp" }
      ]
      environment = [
        { name = "PORT", value = "8080" },
        { name = "AWS_REGION", value = var.aws_region },
        { name = "SKILLS_FILE", value = "/data/skills.yaml" },
        { name = "COMPANIES_FILE", value = "/data/companies.yaml" },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = var.log_group_name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "api"
        }
      }
    }
  ])

  # Terraform only creates the *initial* revision — bootstrapping the
  # service so something exists to deploy onto, possibly before any image
  # has ever been pushed to var.image_tag. Every real deploy after that
  # comes from Jenkins registering a new revision directly against ECS
  # (describe -> swap image -> register -> update-service), not from
  # terraform apply — ignore_changes keeps a routine `terraform apply`
  # from reverting a live deploy back to this placeholder.
  lifecycle {
    ignore_changes = [container_definitions]
  }

  tags = { Name = "${var.project}-api" }
}

resource "aws_ecs_service" "api" {
  name            = "api"
  cluster         = var.ecs_cluster_id
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.public_subnet_ids
    security_groups  = [var.service_api_sg_id]
    assign_public_ip = true # no NAT Gateway (docs/design.md §9 option 3) — needs a public IP to pull from ECR / reach CloudWatch Logs
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.api.arn
    container_name   = "api"
    container_port   = 8080
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200

  # Same rationale as the task definition's ignore_changes: Jenkins owns
  # which revision is live after the first real deploy.
  lifecycle {
    ignore_changes = [task_definition]
  }

  depends_on = [aws_lb_listener.api]

  tags = { Name = "${var.project}-api" }
}

resource "aws_appautoscaling_target" "api" {
  min_capacity       = var.min_capacity
  max_capacity       = var.max_capacity
  resource_id        = "service/${var.ecs_cluster_name}/${aws_ecs_service.api.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}

resource "aws_appautoscaling_policy" "api_cpu" {
  name               = "${var.project}-api-cpu"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.api.resource_id
  scalable_dimension = aws_appautoscaling_target.api.scalable_dimension
  service_namespace  = aws_appautoscaling_target.api.service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value       = 70
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
  }
}
