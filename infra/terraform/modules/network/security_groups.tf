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

  # Jenkins build agents (ephemeral Fargate tasks, §10) connect straight
  # to the controller's private IP on 8080, not through the public ALB —
  # confirmed against a real boot that the ALB's DNS resolves to public
  # IPs even from inside this VPC (no automatic private-IP hairpin for
  # public ALBs), so agent traffic there genuinely traverses the internet
  # gateway and can't be allowlisted by a fixed CIDR (Fargate's public IPs
  # are ephemeral). This SG-to-SG rule, unlike that one, actually works:
  # it's direct private-ENI-to-private-ENI traffic within the VPC.
  ingress {
    description     = "Agent connections from the Jenkins Fargate build agents"
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.fargate.id]
  }

  # EC2 Instance Connect (browser-based SSH from the AWS console) proxies
  # through a per-region AWS-owned IP range, not the operator's own IP -
  # https://ip-ranges.amazonaws.com, service EC2_INSTANCE_CONNECT,
  # us-west-1 = 13.52.6.112/29. Scoped to that /29, not 0.0.0.0/0.
  ingress {
    description = "SSH from the EC2 Instance Connect service range (us-west-1) only"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["13.52.6.112/29"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${var.project}-jenkins-ec2" }
}

# --- Fargate: public-subnet, closed to inbound per §9 option 3. Used by
# Jenkins's ephemeral build agents (Phase 2). ---

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

# --- service-api (Phase 3): API Gateway -> VPC Link -> internal ALB ->
# Fargate. Each hop's SG only accepts traffic from the previous hop's SG -
# no CIDR-based rules, since none of this is meant to be reachable from
# outside the VPC at all (IP-restriction for the whole API happens one
# layer up, at the WAF attached to the API Gateway stage - see
# modules/api-gateway). ---

resource "aws_security_group" "vpc_link" {
  name        = "${var.project}-vpc-link"
  description = "API Gateway VPC Link ENIs - egress only, to the internal ALB"
  vpc_id      = aws_vpc.main.id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${var.project}-vpc-link" }
}

resource "aws_security_group" "api_alb" {
  name        = "${var.project}-api-alb"
  description = "Internal ALB for service-api - inbound only from the VPC Link"
  vpc_id      = aws_vpc.main.id

  ingress {
    description     = "HTTP from the API Gateway VPC Link only"
    from_port       = 80
    to_port         = 80
    protocol        = "tcp"
    security_groups = [aws_security_group.vpc_link.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${var.project}-api-alb" }
}

resource "aws_security_group" "service_api" {
  name        = "${var.project}-service-api"
  description = "service-api ECS tasks - inbound only from the internal ALB"
  vpc_id      = aws_vpc.main.id

  ingress {
    description     = "API traffic from the internal ALB only"
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.api_alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${var.project}-service-api" }
}
