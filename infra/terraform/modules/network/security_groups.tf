# --- Jenkins: never exposed to the open internet (docs/design.md §10) ---

resource "aws_security_group" "jenkins_alb" {
  name        = "${var.project}-jenkins-alb"
  description = "Jenkins ALB - inbound restricted to the operator IP/CIDR only"
  vpc_id      = aws_vpc.main.id

  ingress {
    description = "HTTPS from the allowed operator CIDR only"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = [var.jenkins_allowed_cidr]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${var.project}-jenkins-alb" }
}

resource "aws_security_group" "jenkins_ec2" {
  name        = "${var.project}-jenkins-ec2"
  description = "Jenkins controller - inbound only from its own ALB"
  vpc_id      = aws_vpc.main.id

  ingress {
    description     = "Jenkins UI/API from the ALB only"
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.jenkins_alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${var.project}-jenkins-ec2" }
}

# --- Fargate (Phase 3+): public-subnet, closed to inbound per §9 option 3.
# Not used by anything in Phase 2 - service-api/service-worker come later -
# defined here so this module doesn't need revisiting when they land. ---

resource "aws_security_group" "fargate" {
  name        = "${var.project}-fargate"
  description = "ECS Fargate tasks - public subnet, no inbound, outbound only"
  vpc_id      = aws_vpc.main.id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${var.project}-fargate" }
}
