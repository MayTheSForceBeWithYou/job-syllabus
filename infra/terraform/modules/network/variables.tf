variable "project" {
  type    = string
  default = "job-syllabus"
}

variable "vpc_cidr" {
  description = "Chosen to avoid the account's existing default VPC (172.31.0.0/16)."
  type        = string
  default     = "10.20.0.0/16"
}

variable "azs" {
  description = "docs/design.md §9: 2 AZs. us-west-1 only has 2 usable AZs anyway (1a, 1c)."
  type        = list(string)
  default     = ["us-west-1a", "us-west-1c"]
}

variable "public_subnet_cidrs" {
  type    = list(string)
  default = ["10.20.0.0/24", "10.20.1.0/24"]
}

variable "private_subnet_cidrs" {
  description = "Defined for future use (e.g. a prod env choosing NAT Gateway, docs/design.md §9 option 1). Not used by dev-compute — see enable_nat."
  type        = list(string)
  default     = ["10.20.10.0/24", "10.20.11.0/24"]
}

variable "enable_nat" {
  description = "docs/design.md §9: dev uses option 3 (public-subnet Fargate, no NAT, ~$0/mo) instead of option 1 (NAT Gateway, ~$32/mo). Leave false for dev."
  type        = bool
  default     = false
}

variable "jenkins_allowed_cidr" {
  description = "docs/design.md §10: Jenkins must never be exposed to the open internet. Restricted to the operator's IP or a Tailscale subnet router."
  type        = string
}
