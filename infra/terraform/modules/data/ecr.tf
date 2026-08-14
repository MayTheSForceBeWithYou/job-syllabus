# 5 repos, one per cmd/ binary, per docs/design.md §9.
resource "aws_ecr_repository" "app" {
  for_each = toset(var.ecr_repo_names)

  name                 = "${var.project}/${each.value}"
  image_tag_mutability = "IMMUTABLE" # $GIT_SHA tags per §10's api-build pipeline — never overwrite

  image_scanning_configuration {
    scan_on_push = true
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_ecr_lifecycle_policy" "app" {
  for_each   = aws_ecr_repository.app
  repository = each.value.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Keep the last ${var.ecr_image_retain_count} images"
        selection = {
          tagStatus   = "any"
          countType   = "imageCountMoreThan"
          countNumber = var.ecr_image_retain_count
        }
        action = { type = "expire" }
      }
    ]
  })
}

output "ecr_repository_urls" {
  value = { for k, v in aws_ecr_repository.app : k => v.repository_url }
}
