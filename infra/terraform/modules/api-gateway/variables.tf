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
