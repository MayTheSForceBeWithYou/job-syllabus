variable "project" {
  type    = string
  default = "job-syllabus"
}

variable "log_retention_days" {
  description = "Matches modules/observability's default - unbounded CloudWatch retention is a classic surprise bill (docs/design.md §9)."
  type        = number
  default     = 30
}
