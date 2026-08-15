# Published so the Expo client's build-time config (app.config.ts) and
# ci/Jenkinsfile.client-build can read real, apply-specific values rather
# than hardcoding them — same pattern modules/api-gateway's ssm.tf already
# uses for the API URL. Plain String throughout: a Cognito app client ID
# and a hosted-UI domain aren't secrets (they're public OAuth client
# metadata, visible in any Hosted UI redirect URL).
resource "aws_ssm_parameter" "user_pool_id" {
  name  = "/${var.project}/auth/user_pool_id"
  type  = "String"
  value = aws_cognito_user_pool.main.id
}

resource "aws_ssm_parameter" "user_pool_client_id" {
  name  = "/${var.project}/auth/user_pool_client_id"
  type  = "String"
  value = aws_cognito_user_pool_client.client.id
}

resource "aws_ssm_parameter" "hosted_ui_domain" {
  name  = "/${var.project}/auth/hosted_ui_domain"
  type  = "String"
  value = "${aws_cognito_user_pool_domain.main.domain}.auth.${data.aws_region.current.name}.amazoncognito.com"
}
