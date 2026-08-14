# Long-lived stack: DynamoDB table, S3 data bucket, ECR repos.
# Deliberately NOT destroyed/recreated with dev-compute — see
# docs/design.md §9's amendment for why (these resources are already
# near-free at rest; the cost driver is entirely in dev-compute).
module "data" {
  source = "../../modules/data"
}
