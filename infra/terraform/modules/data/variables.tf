variable "project" {
  description = "Project name, used as a resource-naming prefix."
  type        = string
  default     = "job-syllabus"
}

variable "ecr_repo_names" {
  description = "One ECR repo per cmd/ binary (docs/design.md §9: '5 repos')."
  type        = list(string)
  default     = ["api", "ingest", "scraper", "worker", "rollup"]
}

variable "ecr_image_retain_count" {
  description = "How many images to keep per ECR repo before the lifecycle policy expires older ones."
  type        = number
  default     = 10
}
