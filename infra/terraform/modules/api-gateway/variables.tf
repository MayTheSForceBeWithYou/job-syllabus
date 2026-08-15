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

variable "cognito_issuer_url" {
  type = string
}

variable "cognito_app_client_id" {
  type = string
}

variable "cognito_read_scope" {
  type = string
}

variable "cognito_admin_scope" {
  type = string
}
