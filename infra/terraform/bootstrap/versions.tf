terraform {
  required_version = ">= 1.9"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  # Local backend deliberately - this stack creates the remote backend that
  # every other stack uses. See docs/bootstrap.md.
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project   = "job-syllabus"
      ManagedBy = "terraform"
      Stack     = "bootstrap"
    }
  }
}
