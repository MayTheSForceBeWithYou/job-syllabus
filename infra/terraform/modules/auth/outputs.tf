output "user_pool_id" {
  value = aws_cognito_user_pool.main.id
}

output "user_pool_client_id" {
  value = aws_cognito_user_pool_client.client.id
}

# HTTP APIs' JWT authorizer needs the issuer exactly in this form —
# https://cognito-idp.<region>.amazonaws.com/<user pool id>.
output "issuer_url" {
  value = "https://cognito-idp.${data.aws_region.current.name}.amazonaws.com/${aws_cognito_user_pool.main.id}"
}

output "hosted_ui_domain" {
  value = "${aws_cognito_user_pool_domain.main.domain}.auth.${data.aws_region.current.name}.amazoncognito.com"
}

output "resource_server_identifier" {
  value = aws_cognito_resource_server.api.identifier
}

output "read_scope" {
  value = "${aws_cognito_resource_server.api.identifier}/read"
}

output "admin_scope" {
  value = "${aws_cognito_resource_server.api.identifier}/admin"
}
