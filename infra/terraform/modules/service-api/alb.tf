# Internal (docs/design.md §3 architecture diagram) — never has a public
# IP or a listener reachable outside the VPC. The only thing in front of
# it is the API Gateway VPC Link; IP-restriction for the whole API happens
# one layer further out, at the WAF on the API Gateway stage
# (modules/api-gateway). Plain HTTP is fine here: TLS is terminated by API
# Gateway's own public regional endpoint, and this traffic never leaves
# the VPC.
resource "aws_lb" "api" {
  name               = "${var.project}-api"
  internal           = true
  load_balancer_type = "application"
  subnets            = var.private_subnet_ids
  security_groups    = [var.api_alb_sg_id]

  tags = { Name = "${var.project}-api" }
}

resource "aws_lb_target_group" "api" {
  name        = "${var.project}-api"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip" # Fargate awsvpc mode has no instance to register — IP targets

  health_check {
    path                = "/healthz"
    matcher             = "200"
    interval            = 15
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  tags = { Name = "${var.project}-api" }
}

resource "aws_lb_listener" "api" {
  load_balancer_arn = aws_lb.api.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }
}
