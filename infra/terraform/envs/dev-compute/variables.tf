variable "jenkins_allowed_cidr" {
  description = "docs/design.md §10: Jenkins must never be exposed to the open internet."
  type        = string
}

variable "api_allowed_cidr" {
  description = "docs/design.md §13 Phase 3 DoD: no auth yet, locked to the operator's IP via WAF. Kept distinct from jenkins_allowed_cidr (usually the same value) so the API's access can be widened independently of Jenkins later."
  type        = string
}

variable "alert_email" {
  type = string
}

variable "restore_from_snapshot" {
  description = "false on the very first apply (no DLM snapshot exists yet); true on every apply after that, including specifically after a destroy+recreate — that's the actual Phase 2 DoD scenario."
  type        = bool
  default     = false
}
