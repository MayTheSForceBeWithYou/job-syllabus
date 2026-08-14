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

# Tasks run in public subnets with a public IP, same as Jenkins's Fargate
# build agents - docs/design.md §9 option 3 (no NAT Gateway). The internal
# ALB sits in the private subnets instead: it only ever receives traffic
# from the API Gateway VPC Link within the VPC, never needs to initiate
# outbound internet traffic itself, and has no public IP path anyway
# (internal = true).
variable "public_subnet_ids" {
  type = list(string)
}

variable "private_subnet_ids" {
  type = list(string)
}

variable "service_api_sg_id" {
  type = string
}

variable "api_alb_sg_id" {
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

# Bootstrap-only placeholder — the image at this tag may not even exist on
# a first-ever apply. Every real deploy after that comes from Jenkins
# registering a new task definition revision directly against ECS (not
# via terraform apply); see the task definition's lifecycle.ignore_changes.
variable "image_tag" {
  type    = string
  default = "latest"
}

variable "table_arn" {
  type = string
}

variable "cpu" {
  description = "docs/design.md §9 sizing table: 0.25 vCPU."
  type        = number
  default     = 256
}

variable "memory" {
  description = "docs/design.md §9 sizing table: 512 MB."
  type        = number
  default     = 512
}

variable "desired_count" {
  type    = number
  default = 1
}

variable "min_capacity" {
  type    = number
  default = 1
}

variable "max_capacity" {
  description = "docs/design.md §9 sizing table: 1-3 tasks, CPU > 70%."
  type        = number
  default     = 3
}
