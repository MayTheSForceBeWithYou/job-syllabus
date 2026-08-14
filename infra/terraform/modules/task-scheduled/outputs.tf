output "task_definition_family" {
  value = aws_ecs_task_definition.this.family
}

output "schedule_arn" {
  value = aws_scheduler_schedule.this.arn
}
