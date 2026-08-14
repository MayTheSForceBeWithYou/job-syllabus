variable "project" {
  description = "Project name, used as a resource-naming prefix."
  type        = string
  default     = "job-syllabus"
}

variable "ecr_repo_names" {
  description = "One repo per cmd/ binary (docs/design.md §9: '5 repos') plus 'agent' for the Jenkins ephemeral Fargate build-agent image (ci/agent.Dockerfile, §10) — a 6th repo the doc's count doesn't mention explicitly but its own Jenkins section requires."
  type        = list(string)
  default     = ["api", "ingest", "scraper", "worker", "rollup", "agent"]
}

variable "ecr_image_retain_count" {
  description = "How many images to keep per ECR repo before the lifecycle policy expires older ones."
  type        = number
  default     = 10
}
