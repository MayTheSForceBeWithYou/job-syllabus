output "api_url" {
  value = aws_apigatewayv2_api.main.api_endpoint
}

output "web_acl_arn" {
  value = aws_wafv2_web_acl.api.arn
}
