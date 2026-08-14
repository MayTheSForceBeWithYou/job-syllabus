resource "aws_wafv2_ip_set" "allowed" {
  name               = "${var.project}-api-allowed"
  scope              = "REGIONAL"
  ip_address_version = "IPV4"
  addresses          = [var.operator_allowed_cidr]
}

resource "aws_wafv2_web_acl" "api" {
  name        = "${var.project}-api"
  scope       = "REGIONAL"
  description = "docs/design.md Phase 3 DoD: no auth yet, locked to the operator's IP instead. Default-deny; the one rule allow-lists operator_allowed_cidr."

  default_action {
    block {}
  }

  rule {
    name     = "allow-operator-ip"
    priority = 0

    action {
      allow {}
    }

    statement {
      ip_set_reference_statement {
        arn = aws_wafv2_ip_set.allowed.arn
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${var.project}-api-allow-operator-ip"
      sampled_requests_enabled   = true
    }
  }

  visibility_config {
    cloudwatch_metrics_enabled = true
    metric_name                = "${var.project}-api"
    sampled_requests_enabled   = true
  }

  tags = { Name = "${var.project}-api" }
}

resource "aws_wafv2_web_acl_association" "api" {
  resource_arn = aws_apigatewayv2_stage.default.arn
  web_acl_arn  = aws_wafv2_web_acl.api.arn
}
