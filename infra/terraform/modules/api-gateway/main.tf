# HTTP API -> VPC Link -> internal ALB -> service-api (docs/design.md §3
# architecture diagram). Phase 3's IP-restriction (application-layer
# middleware in cmd/api, since AWS WAFv2 doesn't support associating with
# HTTP APIs at all — see internal/api/ipallow.go) is retired for /v1/* as
# of Phase 6 in favor of the real thing: a Cognito JWT authorizer below.
resource "aws_apigatewayv2_api" "main" {
  name          = "${var.project}-api"
  protocol_type = "HTTP"

  # The Expo web client (CloudFront origin) calls this API directly from
  # the browser via fetch() — confirmed missing by a real preflight
  # failure during Phase 6 DoD validation ("No 'Access-Control-Allow-
  # Origin' header is present"). allow_credentials is deliberately unset:
  # auth is a Bearer token in the Authorization header (internal/api
  # client.ts), not a cookie, so no credentialed-request mode is needed,
  # and pairing allow_credentials=true with a wildcard origin isn't even
  # permitted by the CORS spec anyway.
  cors_configuration {
    allow_origins = [var.web_origin]
    allow_methods = ["GET", "POST", "OPTIONS"]
    allow_headers = ["authorization", "content-type"]
    max_age       = 300
  }
}

# docs/design.md §7: "admin scope for writes, read scope for queries."
# HTTP API JWT authorizers and authorization_scopes attach per route, not
# per path prefix within a catch-all — but every write in this API is a
# POST and every read is a GET (true throughout internal/api, still true
# after Phase 5's POST /v1/reviews/{term}), so splitting the catch-all by
# method below is sufficient without any path-specific logic.
resource "aws_apigatewayv2_authorizer" "cognito" {
  api_id           = aws_apigatewayv2_api.main.id
  authorizer_type  = "JWT"
  identity_sources = ["$request.header.Authorization"]
  name             = "${var.project}-cognito"

  jwt_configuration {
    audience = [var.cognito_app_client_id]
    issuer   = var.cognito_issuer_url
  }
}

resource "aws_apigatewayv2_vpc_link" "main" {
  name               = "${var.project}-vpc-link"
  security_group_ids = [var.vpc_link_sg_id]
  subnet_ids         = var.private_subnet_ids
}

# GET /{proxy+} forwards everything real (currently just /healthz,
# /readyz — every /v1/* path is already fully covered by the more specific
# v1_read/v1_write routes below) to the ALB and lets chi (internal/api) do
# the real routing/404s/method-not-allowed — API Gateway itself stays a
# dumb proxy. payload_format_version 1.0 is what HTTP_PROXY integrations
# to an ALB require.
#
# Deliberately GET, not the original ANY: HTTP APIs only auto-generate a
# CORS preflight (204) response for an OPTIONS request when NO route
# matches it — confirmed by a real preflight from the deployed web client
# coming back 405 "OPTIONS is not supported for /v1/skills" while ANY
# /{proxy+} existed, because that route matched the OPTIONS request too
# and forwarded it straight to chi, which has no OPTIONS handler. Every
# real method actually served today is GET or POST (internal/api/
# server.go), and POST only ever appears under /v1 where the more
# specific v1_write route already owns it, so narrowing this to GET both
# fixes CORS and doesn't drop any route that was reachable before.
#
# request_parameters:
#
# overwrite:path is load-bearing: confirmed against a real smoke test that
# HTTP_PROXY VPC_LINK integrations forward the client's ORIGINAL path
# unchanged, stage prefix included — a request to ".../prod/healthz"
# reached the app as literally "/prod/healthz", which chi correctly 404'd
# since it only knows "/healthz". Rewriting the outgoing path to just the
# captured {proxy} segment strips that prefix.
#
# overwrite:header.x-real-ip is also load-bearing, for a related but
# distinct reason: confirmed against a real request from the operator's
# own allowed IP that X-Forwarded-For does NOT carry the true external
# client IP through a VPC_LINK private integration at all — what actually
# arrives at the backend is the VPC Link's own internal VPC address (the
# hop between the VPC Link and the ALB), not anything traceable back to
# API Gateway's edge. internal/api/ipallow.go's IP-allowlist was unusable
# without this: API Gateway itself DOES know the real client IP
# ($context.identity.sourceIp, already used in this file's access log
# format below), so this injects it into a dedicated header instead of
# relying on X-Forwarded-For. overwrite:, not append:, so a client can't
# supply their own x-real-ip and have it survive alongside/ahead of the
# authoritative value.
resource "aws_apigatewayv2_integration" "alb" {
  api_id                 = aws_apigatewayv2_api.main.id
  integration_type       = "HTTP_PROXY"
  integration_uri        = var.alb_listener_arn
  integration_method     = "ANY"
  connection_type        = "VPC_LINK"
  connection_id          = aws_apigatewayv2_vpc_link.main.id
  payload_format_version = "1.0"

  request_parameters = {
    "overwrite:path"             = "/$request.path.proxy"
    "overwrite:header.x-real-ip" = "$context.identity.sourceIp"
  }
}

resource "aws_apigatewayv2_route" "proxy" {
  api_id    = aws_apigatewayv2_api.main.id
  route_key = "GET /{proxy+}"
  target    = "integrations/${aws_apigatewayv2_integration.alb.id}"
}

# A second integration, identical to the one above except for its
# overwrite:path template — required because {proxy} captures a different
# substring depending on which route matched. For the catch-all's "GET
# /{proxy+}", {proxy} is the whole path (e.g. "healthz"), so rewriting to
# "/$request.path.proxy" is correct. But for "GET /v1/{proxy+}" below,
# {proxy} only captures what's after "/v1/" (e.g. "skills" for a request
# to /v1/skills) — reusing the catch-all's integration here silently
# dropped the "/v1" prefix chi's router requires, confirmed by a real
# request to the deployed API coming back "no route matches /skills" (a
# 404 from chi itself, not from API Gateway) even after CORS was fixed.
# This integration's template hardcodes the "v1/" prefix back on.
resource "aws_apigatewayv2_integration" "alb_v1" {
  api_id                 = aws_apigatewayv2_api.main.id
  integration_type       = "HTTP_PROXY"
  integration_uri        = var.alb_listener_arn
  integration_method     = "ANY"
  connection_type        = "VPC_LINK"
  connection_id          = aws_apigatewayv2_vpc_link.main.id
  payload_format_version = "1.0"

  request_parameters = {
    "overwrite:path"             = "/v1/$request.path.proxy"
    "overwrite:header.x-real-ip" = "$context.identity.sourceIp"
  }
}

# More specific than the ANY catch-all above, so these win route
# resolution for anything under /v1 — HTTP APIs pick the most specific
# matching route. /healthz and /readyz (no {proxy+} match here) keep
# falling through to the unauthenticated catch-all above, unchanged from
# Phase 3.
resource "aws_apigatewayv2_route" "v1_read" {
  api_id    = aws_apigatewayv2_api.main.id
  route_key = "GET /v1/{proxy+}"
  target    = "integrations/${aws_apigatewayv2_integration.alb_v1.id}"

  authorization_type   = "JWT"
  authorizer_id        = aws_apigatewayv2_authorizer.cognito.id
  authorization_scopes = [var.cognito_read_scope]
}

resource "aws_apigatewayv2_route" "v1_write" {
  api_id    = aws_apigatewayv2_api.main.id
  route_key = "POST /v1/{proxy+}"
  target    = "integrations/${aws_apigatewayv2_integration.alb_v1.id}"

  authorization_type   = "JWT"
  authorizer_id        = aws_apigatewayv2_authorizer.cognito.id
  authorization_scopes = [var.cognito_admin_scope]
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
