# 5 repos, one per cmd/ binary, per docs/design.md §9.
resource "aws_ecr_repository" "app" {
  for_each = toset(var.ecr_repo_names)

  name = "${var.project}/${each.value}"
  # Every app image (api, ingest, worker, scraper, rollup) is tagged with
  # $GIT_SHA per §10's api-build pipeline and stays IMMUTABLE for a real
  # deploy audit trail. "agent" is different: it's Jenkins's own build
  # tooling image, always referenced as :latest (ci/jenkins.yaml), and
  # expected to be rebuilt/repushed under that same tag as the Dockerfile
  # evolves — an immutable :latest can never be re-pushed at all, which a
  # real rebuild hit directly ("tag invalid... immutable").
  image_tag_mutability = each.value == "agent" ? "MUTABLE" : "IMMUTABLE"

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
