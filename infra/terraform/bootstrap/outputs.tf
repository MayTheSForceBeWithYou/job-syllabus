output "state_bucket" {
  value       = aws_s3_bucket.tfstate.id
  description = "S3 bucket name — commit this into envs/*/backend.tf per docs/bootstrap.md."
}

output "lock_table" {
  value       = aws_dynamodb_table.tflock.name
  description = "DynamoDB table name for state locking — commit this into envs/*/backend.tf."
}

output "region" {
  value = var.region
}
