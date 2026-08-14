output "cluster_id" {
  value = aws_ecs_cluster.main.id
}

output "cluster_name" {
  value = aws_ecs_cluster.main.name
}

output "task_execution_role_arn" {
  value = aws_iam_role.task_execution.arn
}

output "log_group_name" {
  value = aws_cloudwatch_log_group.cluster.name
}
