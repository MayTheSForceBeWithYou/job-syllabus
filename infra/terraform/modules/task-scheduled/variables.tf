variable "project" {
  type    = string
  default = "job-syllabus"
}

variable "aws_region" {
  type = string
}

# Distinguishes this scheduled task from others reusing this same module
# (e.g. "ingest-daily", "rollup-daily", "rollup-reconcile") — becomes part
# of the task definition family and schedule name, so it must be unique
# across every instantiation.
variable "name" {
  type = string
}

variable "ecr_repo_url" {
  type = string
}

# Bootstrap-only placeholder, same rationale as modules/service-api's
# image_tag.
variable "image_tag" {
  type    = string
  default = "latest"
}

# Overrides the image's ENTRYPOINT args — e.g. ["ingest"] or ["reconcile"]
# for cmd/ingest/cmd/rollup's subcommand-style CLIs.
variable "command" {
  type = list(string)
}

variable "environment" {
  type    = list(object({ name = string, value = string }))
  default = []
}

variable "cpu" {
  type    = number
  default = 512
}

variable "memory" {
  type    = number
  default = 1024
}

variable "ecs_cluster_name" {
  type = string
}

variable "task_execution_role_arn" {
  type = string
}

# Distinct per instantiation — cmd/ingest's producer role differs from
# cmd/rollup's backup/reconcile role, unlike modules/service-worker where
# one task role covers everything cmd/worker's single binary does.
variable "task_role_arn" {
  type = string
}

variable "subnet_ids" {
  type = list(string)
}

variable "security_group_ids" {
  type = list(string)
}

variable "log_group_name" {
  type = string
}

# EventBridge Scheduler's own cron/rate syntax, e.g.
# "cron(0 6 * * ? *)" for daily 06:00 UTC (docs/design.md §5's scheduling
# section).
variable "schedule_expression" {
  type = string
}
