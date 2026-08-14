variable "region" {
  description = "AWS region for all job-syllabus infrastructure."
  type        = string
  default     = "us-west-1"
}

variable "budget_limit_usd" {
  description = "Monthly cost budget threshold (docs/design.md §12: set before Phase 2, not after)."
  type        = number
  default     = 60
}

variable "alert_email" {
  description = "Email address for budget threshold notifications."
  type        = string
}
