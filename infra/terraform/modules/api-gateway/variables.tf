variable "project" {
  type    = string
  default = "job-syllabus"
}

variable "vpc_link_sg_id" {
  type = string
}

variable "private_subnet_ids" {
  type = list(string)
}

variable "alb_listener_arn" {
  type = string
}

# docs/design.md §13 Phase 3 DoD: "Read endpoints only, no auth yet
# (locked to your IP)." HTTP APIs (API Gateway v2) don't support resource
# policies the way REST APIs (v1) do, so IP-restriction happens via a WAF
# WebACL on the stage instead — see waf.tf. The Cognito JWT authorizer
# that replaces this is Phase 6.
variable "operator_allowed_cidr" {
  type = string
}
