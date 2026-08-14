terraform {
  required_version = ">= 1.9"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project   = "job-syllabus"
      ManagedBy = "terraform"
      Stack     = "dev-data"
    }
  }
}

variable "region" {
  type    = string
  default = "us-west-1"
}
