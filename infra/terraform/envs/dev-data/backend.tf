# Bucket/table names match infra/terraform/bootstrap's outputs exactly.
# See docs/bootstrap.md for the chicken-and-egg reasoning.
terraform {
  backend "s3" {
    bucket         = "job-syllabus-tfstate-881811711506"
    key            = "dev-data/terraform.tfstate"
    region         = "us-west-1"
    dynamodb_table = "job-syllabus-tfstate-lock"
    encrypt        = true
  }
}
