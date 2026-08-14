output "vpc_id" {
  value = aws_vpc.main.id
}

output "public_subnet_ids" {
  value = aws_subnet.public[*].id
}

output "private_subnet_ids" {
  value = aws_subnet.private[*].id
}

output "jenkins_alb_sg_id" {
  value = aws_security_group.jenkins_alb.id
}

output "jenkins_ec2_sg_id" {
  value = aws_security_group.jenkins_ec2.id
}

output "fargate_sg_id" {
  value = aws_security_group.fargate.id
}

output "vpc_link_sg_id" {
  value = aws_security_group.vpc_link.id
}

output "api_alb_sg_id" {
  value = aws_security_group.api_alb.id
}

output "service_api_sg_id" {
  value = aws_security_group.service_api.id
}
