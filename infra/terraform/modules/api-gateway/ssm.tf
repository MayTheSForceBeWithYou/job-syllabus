# Published so ci/Jenkinsfile.api-build's smoke-test stage can read the
# real URL rather than hardcoding it — same "dynamic, apply-specific value
# via SSM" pattern the Jenkins admin password already uses. Plain String,
# not SecureString: this isn't a secret.
resource "aws_ssm_parameter" "api_url" {
  name  = "/${var.project}/api/url"
  type  = "String"
  value = "${aws_apigatewayv2_api.main.api_endpoint}/prod"
}
