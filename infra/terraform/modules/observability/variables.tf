variable "project" {
  type    = string
  default = "job-syllabus"
}

variable "alert_email" {
  description = "Email for the SNS alert topic (DLQ depth, etc)."
  type        = string
}

variable "dlq_names" {
  description = "map(queueName => dlqName) — CloudWatch alarms need the queue name, not the ARN, for the QueueName dimension."
  type        = map(string)
}
