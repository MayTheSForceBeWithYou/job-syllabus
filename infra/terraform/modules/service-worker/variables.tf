variable "project" {
  type    = string
  default = "job-syllabus"
}

variable "aws_region" {
  type = string
}

variable "vpc_id" {
  type = string
}

# Same no-NAT, public-subnet-with-public-IP pattern service-api and the
# Jenkins Fargate build agents already use (docs/design.md §9 option 3) —
# the worker has no inbound listener at all (nothing calls into it; it
# only consumes SQS and calls out to S3/DynamoDB/SQS), so it reuses the
# general-purpose `fargate` security group rather than needing its own.
variable "public_subnet_ids" {
  type = list(string)
}

variable "fargate_sg_id" {
  type = string
}

variable "ecs_cluster_id" {
  type = string
}

variable "ecs_cluster_name" {
  type = string
}

variable "task_execution_role_arn" {
  type = string
}

variable "log_group_name" {
  type = string
}

variable "ecr_repo_url" {
  type = string
}

# Bootstrap-only placeholder, same rationale as modules/service-api's
# image_tag — see this module's ecs.tf lifecycle.ignore_changes.
variable "image_tag" {
  type    = string
  default = "latest"
}

variable "table_arn" {
  type = string
}

variable "raw_bucket_arn" {
  type = string
}

variable "extract_queue_url" {
  type = string
}

variable "extract_queue_arn" {
  type = string
}

variable "cpu" {
  description = "docs/design.md §9 sizing table: 0.5 vCPU."
  type        = number
  default     = 512
}

variable "memory" {
  description = "docs/design.md §9 sizing table: 1 GB."
  type        = number
  default     = 1024
}

# docs/design.md §9 sizing table: "0-4 tasks scaled on SQS
# ApproximateNumberOfMessagesVisible." min_capacity=0 (not 1, unlike
# service-api) is deliberate — an empty queue means zero postings waiting
# on extraction, so there's nothing useful for a running task to do, and
# Fargate Spot-eligible workloads are exactly what §9's cost guidance
# wants scaled to zero rather than idling.
variable "min_capacity" {
  type    = number
  default = 0
}

variable "max_capacity" {
  type    = number
  default = 4
}

# Alarm thresholds driving the two step-scaling policies below (ecs.tf).
variable "scale_out_queue_depth" {
  description = "ApproximateNumberOfMessagesVisible above which to add a task."
  type        = number
  default     = 10
}

variable "scale_in_evaluation_periods" {
  description = "Consecutive 1-minute periods of an empty queue before scaling to zero — avoids flapping a task up and back down between two closely-spaced ingest runs."
  type        = number
  default     = 5
}
