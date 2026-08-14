# Long-lived stack: DynamoDB table, S3 data bucket, ECR repos, DNS zone.
# Deliberately NOT destroyed/recreated with dev-compute — see
# docs/design.md §9's amendment for why (these resources are already
# near-free at rest; the cost driver is entirely in dev-compute).
module "data" {
  source = "../../modules/data"
}

# Persistent so the one-time Squarespace NS delegation (for skopekreep.com)
# never has to be redone when dev-compute (and the ACM cert/ALB that
# actually use this zone) gets destroyed/recreated for cost control.
module "dns" {
  source    = "../../modules/dns"
  subdomain = "job-syllabus.skopekreep.com"
}
