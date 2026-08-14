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
