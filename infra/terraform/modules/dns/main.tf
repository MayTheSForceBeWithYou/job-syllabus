# Persistent by design (lives in envs/dev-data, not dev-compute): this zone
# requires a one-time manual NS delegation at the domain's DNS host
# (Squarespace, for skopekreep.com). If this zone lived in dev-compute, that
# manual step would have to be redone every time the operator destroys and
# recreates the compute stack for cost control - defeating the point.
resource "aws_route53_zone" "subdomain" {
  name = var.subdomain

  comment = "job-syllabus - delegated subdomain, apex domain's own DNS/hosting is untouched"

  lifecycle {
    prevent_destroy = true
  }
}
