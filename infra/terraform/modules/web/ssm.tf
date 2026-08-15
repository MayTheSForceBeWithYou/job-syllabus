# Same "dynamic, apply-specific value via SSM" pattern as modules/api-gateway
# and modules/auth — ci/Jenkinsfile.client-build reads these rather than
# hardcoding a bucket name or distribution ID.
resource "aws_ssm_parameter" "bucket_name" {
  name  = "/${var.project}/web/bucket_name"
  type  = "String"
  value = aws_s3_bucket.web.id
}

resource "aws_ssm_parameter" "distribution_id" {
  name  = "/${var.project}/web/distribution_id"
  type  = "String"
  value = aws_cloudfront_distribution.web.id
}

resource "aws_ssm_parameter" "url" {
  name  = "/${var.project}/web/url"
  type  = "String"
  value = "https://${aws_cloudfront_distribution.web.domain_name}"
}
