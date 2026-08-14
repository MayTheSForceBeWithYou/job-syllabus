# Gateway endpoints are free and make S3/DynamoDB reachable without NAT —
# the core of docs/design.md §9's no-NAT dev recommendation. Interface
# endpoints (ECR, CloudWatch Logs, SQS, Secrets Manager) are deliberately
# NOT added: at ~$7/mo each, a handful of them would exceed the NAT
# Gateway they're meant to help avoid (§9's own math). Public-subnet
# Fargate with a public IP reaches those services directly instead.
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.main.id
  service_name      = "com.amazonaws.${data.aws_region.current.name}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = concat([aws_route_table.public.id], var.enable_nat ? [aws_route_table.private.id] : [])

  tags = { Name = "${var.project}-s3" }
}

resource "aws_vpc_endpoint" "dynamodb" {
  vpc_id            = aws_vpc.main.id
  service_name      = "com.amazonaws.${data.aws_region.current.name}.dynamodb"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = concat([aws_route_table.public.id], var.enable_nat ? [aws_route_table.private.id] : [])

  tags = { Name = "${var.project}-dynamodb" }
}

data "aws_region" "current" {}
