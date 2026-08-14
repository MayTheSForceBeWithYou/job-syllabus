# HTTP API -> VPC Link -> internal ALB -> service-api (docs/design.md §3
# architecture diagram). No JWT authorizer yet (Phase 6) - IP-restriction
# for Phase 3 is application-layer middleware in cmd/api instead of a WAF
# here, since AWS WAFv2 doesn't support associating with HTTP APIs at all
# (only REST APIs) — see internal/api/ipallow.go for the full rationale.
resource "aws_apigatewayv2_api" "main" {
  name          = "${var.project}-api"
  protocol_type = "HTTP"
}

resource "aws_apigatewayv2_vpc_link" "main" {
  name               = "${var.project}-vpc-link"
  security_group_ids = [var.vpc_link_sg_id]
  subnet_ids         = var.private_subnet_ids
}

# ANY /{proxy+} forwards everything to the ALB and lets chi (internal/api)
# do the real routing/404s/method-not-allowed — API Gateway itself stays a
# dumb proxy. payload_format_version 1.0 is what HTTP_PROXY integrations
# to an ALB require.
resource "aws_apigatewayv2_integration" "alb" {
  api_id                 = aws_apigatewayv2_api.main.id
  integration_type       = "HTTP_PROXY"
  integration_uri        = var.alb_listener_arn
  integration_method     = "ANY"
  connection_type        = "VPC_LINK"
  connection_id          = aws_apigatewayv2_vpc_link.main.id
  payload_format_version = "1.0"
}

resource "aws_apigatewayv2_route" "proxy" {
  api_id    = aws_apigatewayv2_api.main.id
  route_key = "ANY /{proxy+}"
  target    = "integrations/${aws_apigatewayv2_integration.alb.id}"
}

resource "aws_cloudwatch_log_group" "access" {
  name              = "/apigateway/${var.project}-api"
  retention_in_days = 30
}

# A named stage ("prod"), not HTTP API's auto-generated "$default" pseudo
# -stage: WAFv2's AssociateWebACL rejects a $default stage's ARN outright
# ("The ARN isn't valid"), even percent-encoded — confirmed against a real
# apply, not just docs. A named stage sidesteps the whole class of issue
# and is the more conventional setup anyway. This does mean the API's
# base path is .../prod, not the bare domain — see outputs.tf/ssm.tf.
resource "aws_apigatewayv2_stage" "prod" {
  api_id      = aws_apigatewayv2_api.main.id
  name        = "prod"
  auto_deploy = true

  access_log_settings {
    destination_arn = aws_cloudwatch_log_group.access.arn
    format = jsonencode({
      requestId      = "$context.requestId"
      ip             = "$context.identity.sourceIp"
      httpMethod     = "$context.httpMethod"
      path           = "$context.path"
      status         = "$context.status"
      responseLength = "$context.responseLength"
      integrationErr = "$context.integrationErrorMessage"
    })
  }
}
