terraform {
  required_version = ">= 1.9"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project   = "job-syllabus"
      ManagedBy = "terraform"
      Stack     = "dev-compute"
    }
  }
}

variable "region" {
  type    = string
  default = "us-west-1"
}
