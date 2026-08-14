variable "project" {
  type    = string
  default = "job-syllabus"
}

variable "max_receive_count" {
  description = "Deliveries before a message goes to its DLQ."
  type        = number
  default     = 3
}
