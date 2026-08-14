data "aws_subnet" "jenkins" {
  id = var.public_subnet_ids[0]
}

data "aws_ssm_parameter" "al2023_arm" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64"
}

resource "aws_instance" "jenkins" {
  ami                    = data.aws_ssm_parameter.al2023_arm.value
  instance_type          = "t4g.small" # ARM Graviton, ~$12/mo — docs/design.md §10
  subnet_id              = var.public_subnet_ids[0]
  availability_zone      = data.aws_subnet.jenkins.availability_zone
  vpc_security_group_ids = [var.jenkins_ec2_sg_id]
  iam_instance_profile   = aws_iam_instance_profile.jenkins.name

  # IMDSv2 enforced — docs/design.md §11.
  metadata_options {
    http_tokens   = "required"
    http_endpoint = "enabled"
  }

  root_block_device {
    volume_type = "gp3"
    volume_size = 12 # OS + Jenkins WAR + plugin jars; JENKINS_HOME itself is the separate volume
  }

  user_data = templatefile("${path.module}/user_data.sh.tftpl", {
    repo_url                = var.repo_url
    admin_password_ssm_name = aws_ssm_parameter.jenkins_admin_password.name
    aws_region              = data.aws_region.current.name
    ecs_cluster_arn         = var.ecs_cluster_arn
    agent_ecr_repo_url      = var.agent_ecr_repo_url
    fargate_subnet_ids      = join(",", var.public_subnet_ids)
    fargate_sg_id           = var.fargate_sg_id
    task_execution_role_arn = var.task_execution_role_arn
    agent_task_role_arn     = aws_iam_role.jenkins_agent.arn
  })
  user_data_replace_on_change = true

  tags = { Name = "${var.project}-jenkins" }
}
