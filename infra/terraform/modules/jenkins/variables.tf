variable "project" {
  type    = string
  default = "job-syllabus"
}

variable "vpc_id" {
  type = string
}

variable "public_subnet_ids" {
  description = "At least 2, for the ALB. The EC2 instance itself uses the first one."
  type        = list(string)
}

variable "jenkins_alb_sg_id" {
  type = string
}

variable "jenkins_ec2_sg_id" {
  type = string
}

variable "dns_zone_id" {
  description = "Route53 zone ID from envs/dev-data's dns module output."
  type        = string
}

variable "dns_subdomain" {
  description = "e.g. job-syllabus.skopekreep.com — Jenkins gets jenkins.<this>."
  type        = string
}

variable "repo_url" {
  description = "Public git URL user_data clones to get ci/jenkins.yaml, ci/plugins.txt, ci/jobs.groovy at boot."
  type        = string
  default     = "https://github.com/MayTheSForceBeWithYou/job-syllabus.git"
}

variable "restore_from_snapshot" {
  description = "Set true once a DLM snapshot of the Jenkins EBS volume exists (i.e. not the very first apply), to restore JENKINS_HOME from the latest one. Leave false when no snapshot exists yet — the data source errors if it finds zero matches, so this can't be auto-detected safely."
  type        = bool
  default     = false
}

variable "ebs_size_gb" {
  type    = number
  default = 20
}

# --- Passed through to JCasC (ci/jenkins.yaml) via user_data, for the
# Amazon ECS plugin's ephemeral Fargate agent cloud config (§10). ---

variable "ecs_cluster_arn" {
  type = string
}

variable "agent_ecr_repo_url" {
  description = "ECR repo URL for the Jenkins agent image (ci/agent.Dockerfile)."
  type        = string
}

variable "kaniko_agent_ecr_repo_url" {
  description = "ECR repo URL for the dedicated Kaniko build agent image (ci/kaniko-agent.Dockerfile) — split out from agent_ecr_repo_url after Kaniko corrupted the general-purpose agent's shared filesystem mid-build."
  type        = string
}

variable "fargate_sg_id" {
  type = string
}

variable "task_execution_role_arn" {
  description = "From modules/ecs-cluster — the Fargate agent tasks' execution role."
  type        = string
}
