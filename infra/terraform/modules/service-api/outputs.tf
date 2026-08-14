output "alb_listener_arn" {
  value = aws_lb_listener.api.arn
}

output "alb_dns_name" {
  value = aws_lb.api.dns_name
}

output "service_name" {
  value = aws_ecs_service.api.name
}

output "task_definition_family" {
  value = aws_ecs_task_definition.api.family
}

output "task_role_arn" {
  value = aws_iam_role.task.arn
}
