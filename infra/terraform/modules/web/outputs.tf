output "bucket_name" {
  value = aws_s3_bucket.web.id
}

output "distribution_id" {
  value = aws_cloudfront_distribution.web.id
}

output "domain_name" {
  value = aws_cloudfront_distribution.web.domain_name
}

output "url" {
  value = "https://${aws_cloudfront_distribution.web.domain_name}"
}
