# Ephemeral stack: network, ecs-cluster, queues, jenkins, observability.
# This is what gets destroyed/applied at will for cost control — see
# docs/design.md §9's amendment. Reads envs/dev-data's outputs (table,
# buckets, ECR repos, DNS zone) via remote state instead of owning them.
data "terraform_remote_state" "data" {
  backend = "s3"
  config = {
    bucket = "job-syllabus-tfstate-881811711506"
    key    = "dev-data/terraform.tfstate"
    region = "us-west-1"
  }
}

module "network" {
  source               = "../../modules/network"
  jenkins_allowed_cidr = var.jenkins_allowed_cidr
}

module "ecs_cluster" {
  source = "../../modules/ecs-cluster"
}

module "queues" {
  source = "../../modules/queues"
}

module "observability" {
  source      = "../../modules/observability"
  alert_email = var.alert_email
  dlq_names   = module.queues.dlq_names
}

module "jenkins" {
  source = "../../modules/jenkins"

  vpc_id                    = module.network.vpc_id
  public_subnet_ids         = module.network.public_subnet_ids
  jenkins_alb_sg_id         = module.network.jenkins_alb_sg_id
  jenkins_ec2_sg_id         = module.network.jenkins_ec2_sg_id
  dns_zone_id               = data.terraform_remote_state.data.outputs.dns_zone_id
  dns_subdomain             = data.terraform_remote_state.data.outputs.dns_subdomain
  restore_from_snapshot     = var.restore_from_snapshot
  ecs_cluster_arn           = module.ecs_cluster.cluster_arn
  agent_ecr_repo_url        = data.terraform_remote_state.data.outputs.ecr_repository_urls["agent"]
  kaniko_agent_ecr_repo_url = data.terraform_remote_state.data.outputs.ecr_repository_urls["kaniko-agent"]
  fargate_sg_id             = module.network.fargate_sg_id
  task_execution_role_arn   = module.ecs_cluster.task_execution_role_arn
}

module "service_api" {
  source = "../../modules/service-api"

  aws_region              = "us-west-1"
  vpc_id                  = module.network.vpc_id
  public_subnet_ids       = module.network.public_subnet_ids
  private_subnet_ids      = module.network.private_subnet_ids
  service_api_sg_id       = module.network.service_api_sg_id
  api_alb_sg_id           = module.network.api_alb_sg_id
  ecs_cluster_id          = module.ecs_cluster.cluster_id
  ecs_cluster_name        = module.ecs_cluster.cluster_name
  task_execution_role_arn = module.ecs_cluster.task_execution_role_arn
  log_group_name          = module.ecs_cluster.log_group_name
  ecr_repo_url            = data.terraform_remote_state.data.outputs.ecr_repository_urls["api"]
  table_arn               = data.terraform_remote_state.data.outputs.table_arn
}

# docs/design.md §8: Expo native scheme + the web deploy's real CloudFront
# domain (module.web, below) + localhost for `expo start --web` dev — all
# three need to be Hosted-UI-registered callback targets since a single
# Cognito app client is shared across every target this one Expo codebase
# builds for (docs/design.md §8: "one Expo codebase"). "jobsyllabus://",
# bare scheme with no path — expo-auth-session's makeRedirectUri()
# explicitly does NOT append a `path` option on native, only on web/Expo
# Go, confirmed against the current SDK 57 docs before wiring
# mobile/src/auth; an earlier "jobsyllabus://redirect" here would never
# have matched what the app actually requests.
module "auth" {
  source = "../../modules/auth"

  callback_urls = [
    "jobsyllabus://",
    "https://${module.web.domain_name}",
    "http://localhost:8081",
  ]
  logout_urls = [
    "jobsyllabus://",
    "https://${module.web.domain_name}",
    "http://localhost:8081",
  ]
}

module "web" {
  source = "../../modules/web"
}

module "api_gateway" {
  source = "../../modules/api-gateway"

  vpc_link_sg_id     = module.network.vpc_link_sg_id
  private_subnet_ids = module.network.private_subnet_ids
  alb_listener_arn   = module.service_api.alb_listener_arn

  cognito_issuer_url    = module.auth.issuer_url
  cognito_app_client_id = module.auth.user_pool_client_id
  cognito_read_scope    = module.auth.read_scope
  cognito_admin_scope   = module.auth.admin_scope

  web_origin = module.web.url
}

module "service_worker" {
  source = "../../modules/service-worker"

  aws_region              = "us-west-1"
  vpc_id                  = module.network.vpc_id
  public_subnet_ids       = module.network.public_subnet_ids
  fargate_sg_id           = module.network.fargate_sg_id
  ecs_cluster_id          = module.ecs_cluster.cluster_id
  ecs_cluster_name        = module.ecs_cluster.cluster_name
  task_execution_role_arn = module.ecs_cluster.task_execution_role_arn
  log_group_name          = module.ecs_cluster.log_group_name
  ecr_repo_url            = data.terraform_remote_state.data.outputs.ecr_repository_urls["worker"]
  table_arn               = data.terraform_remote_state.data.outputs.table_arn
  raw_bucket_arn          = data.terraform_remote_state.data.outputs.data_bucket_arn
  extract_queue_url       = module.queues.queue_urls["extract"]
  extract_queue_arn       = module.queues.queue_arns["extract"]
}

# EventBridge Scheduler -> ecs:RunTask on cmd/ingest, daily 06:00 UTC
# (docs/design.md §5 "Scheduling") — the recurring path; Jenkins'
# parameterized backfill job (ci/jobs.groovy) covers on-demand single-
# company re-ingestion instead of overloading this schedule for it.
module "task_scheduled_ingest" {
  source = "../../modules/task-scheduled"

  aws_region   = "us-west-1"
  name         = "ingest"
  ecr_repo_url = data.terraform_remote_state.data.outputs.ecr_repository_urls["ingest"]
  command      = ["ingest"]
  environment = [
    { name = "COMPANIES_FILE", value = "/data/companies.yaml" },
    { name = "SKILLS_FILE", value = "/data/skills.yaml" },
    { name = "RAW_BUCKET", value = data.terraform_remote_state.data.outputs.data_bucket },
    { name = "EXTRACT_QUEUE_URL", value = module.queues.queue_urls["extract"] },
  ]
  ecs_cluster_name        = module.ecs_cluster.cluster_name
  task_execution_role_arn = module.ecs_cluster.task_execution_role_arn
  task_role_arn           = aws_iam_role.ingest_task.arn
  subnet_ids              = module.network.public_subnet_ids
  security_group_ids      = [module.network.fargate_sg_id]
  log_group_name          = module.ecs_cluster.log_group_name
  schedule_expression     = "cron(0 6 * * ? *)"
}

# EventBridge Scheduler -> ecs:RunTask on `cmd/rollup reconcile`, daily
# 07:00 UTC (docs/design.md §4/§5) — an hour after ingest so extraction
# from that day's run has had time to drain through cmd/worker's queue
# first; reconciling against a still-in-flight backlog would just find
# (correct, temporary) drift and immediately "fix" counters that were
# about to update on their own anyway.
module "task_scheduled_rollup_reconcile" {
  source = "../../modules/task-scheduled"

  aws_region   = "us-west-1"
  name         = "rollup-reconcile"
  ecr_repo_url = data.terraform_remote_state.data.outputs.ecr_repository_urls["rollup"]
  command      = ["reconcile"]
  environment = [
    { name = "BACKUP_BUCKET", value = data.terraform_remote_state.data.outputs.data_bucket },
    # Unused by `reconcile` itself — present so the `reextract` job (Phase 5,
    # ci/Jenkinsfile.reextract) can run against this same task definition
    # family via `ecs run-task --overrides` (command override only, not
    # environment) without a second Terraform-managed task definition.
    { name = "EXTRACT_QUEUE_URL", value = module.queues.queue_urls["extract"] },
  ]
  ecs_cluster_name        = module.ecs_cluster.cluster_name
  task_execution_role_arn = module.ecs_cluster.task_execution_role_arn
  task_role_arn           = aws_iam_role.rollup_task.arn
  subnet_ids              = module.network.public_subnet_ids
  security_group_ids      = [module.network.fargate_sg_id]
  log_group_name          = module.ecs_cluster.log_group_name
  schedule_expression     = "cron(0 7 * * ? *)"
}
