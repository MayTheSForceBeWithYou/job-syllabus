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

  vpc_id                  = module.network.vpc_id
  public_subnet_ids       = module.network.public_subnet_ids
  jenkins_alb_sg_id       = module.network.jenkins_alb_sg_id
  jenkins_ec2_sg_id       = module.network.jenkins_ec2_sg_id
  dns_zone_id             = data.terraform_remote_state.data.outputs.dns_zone_id
  dns_subdomain           = data.terraform_remote_state.data.outputs.dns_subdomain
  restore_from_snapshot   = var.restore_from_snapshot
  ecs_cluster_arn         = module.ecs_cluster.cluster_arn
  agent_ecr_repo_url      = data.terraform_remote_state.data.outputs.ecr_repository_urls["agent"]
  fargate_sg_id           = module.network.fargate_sg_id
  task_execution_role_arn = module.ecs_cluster.task_execution_role_arn
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
  allowed_cidr            = var.api_allowed_cidr
}

module "api_gateway" {
  source = "../../modules/api-gateway"

  vpc_link_sg_id     = module.network.vpc_link_sg_id
  private_subnet_ids = module.network.private_subnet_ids
  alb_listener_arn   = module.service_api.alb_listener_arn
}
