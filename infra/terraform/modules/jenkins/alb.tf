locals {
  jenkins_hostname = "jenkins.${var.dns_subdomain}"
}

resource "aws_acm_certificate" "jenkins" {
  domain_name       = local.jenkins_hostname
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "jenkins_cert_validation" {
  for_each = {
    for dvo in aws_acm_certificate.jenkins.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  zone_id         = var.dns_zone_id
  name            = each.value.name
  type            = each.value.type
  records         = [each.value.record]
  ttl             = 300
  allow_overwrite = true
}

resource "aws_acm_certificate_validation" "jenkins" {
  certificate_arn         = aws_acm_certificate.jenkins.arn
  validation_record_fqdns = [for r in aws_route53_record.jenkins_cert_validation : r.fqdn]
}

resource "aws_route53_record" "jenkins" {
  zone_id = var.dns_zone_id
  name    = local.jenkins_hostname
  type    = "A"

  alias {
    name                   = aws_lb.jenkins.dns_name
    zone_id                = aws_lb.jenkins.zone_id
    evaluate_target_health = true
  }
}

resource "aws_lb" "jenkins" {
  name               = "${var.project}-jenkins"
  internal           = false # SG restricts who can reach it — see modules/network
  load_balancer_type = "application"
  security_groups    = [var.jenkins_alb_sg_id]
  subnets            = var.public_subnet_ids

  tags = { Name = "${var.project}-jenkins" }
}

resource "aws_lb_target_group" "jenkins" {
  name     = "${var.project}-jenkins"
  port     = 8080
  protocol = "HTTP"
  vpc_id   = var.vpc_id

  health_check {
    path                = "/login"
    matcher             = "200-499" # Jenkins may 403 unauthenticated depending on security config — still "up"
    interval            = 30
    healthy_threshold   = 2
    unhealthy_threshold = 5
  }

  target_type = "instance"
}

resource "aws_lb_target_group_attachment" "jenkins" {
  target_group_arn = aws_lb_target_group.jenkins.arn
  target_id        = aws_instance.jenkins.id
  port             = 8080
}

resource "aws_lb_listener" "jenkins_https" {
  load_balancer_arn = aws_lb.jenkins.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = aws_acm_certificate_validation.jenkins.certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.jenkins.arn
  }
}

resource "aws_lb_listener" "jenkins_http_redirect" {
  load_balancer_arn = aws_lb.jenkins.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}
