terraform {
  backend "s3" {
    bucket         = "job-syllabus-tfstate-881811711506"
    key            = "dev-compute/terraform.tfstate"
    region         = "us-west-1"
    dynamodb_table = "job-syllabus-tfstate-lock"
    encrypt        = true
  }
}
