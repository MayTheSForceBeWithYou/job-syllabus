output "url" {
  value = "https://${local.jenkins_hostname}"
}

output "instance_id" {
  value = aws_instance.jenkins.id
}

output "role_arn" {
  value = aws_iam_role.jenkins.arn
}

output "ebs_volume_id" {
  value = aws_ebs_volume.jenkins_home.id
}
