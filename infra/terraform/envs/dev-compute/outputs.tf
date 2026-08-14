output "jenkins_url" {
  value = module.jenkins.url
}

output "jenkins_admin_password_ssm_path" {
  value       = "/job-syllabus/jenkins/admin-password"
  description = "Retrieve with: aws ssm get-parameter --name <this> --with-decryption --region us-west-1 --query Parameter.Value --output text"
}

output "vpc_id" {
  value = module.network.vpc_id
}

output "ecs_cluster_name" {
  value = module.ecs_cluster.cluster_name
}

output "queue_urls" {
  value = module.queues.queue_urls
}

output "alerts_topic_arn" {
  value = module.observability.alerts_topic_arn
}

output "api_url" {
  value = module.api_gateway.api_url
}
