variable "project" {
  type    = string
  default = "job-syllabus"
}

variable "callback_urls" {
  description = "Hosted UI redirect targets — the Expo native scheme, the web deploy's CloudFront domain, and localhost for `expo start --web` dev."
  type        = list(string)
}

variable "logout_urls" {
  description = "Where Hosted UI sends the browser after sign-out."
  type        = list(string)
}
