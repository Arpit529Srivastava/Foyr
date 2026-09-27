locals {
  platform_repos = ["foyrd", "authproxy", "web"]
}

# Images for Foyr's own services; customer repos (apps/<app>) are created by foyrd.
resource "aws_ecr_repository" "platform" {
  for_each             = toset(local.platform_repos)
  name                 = "foyr/${each.key}"
  image_tag_mutability = "IMMUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_lifecycle_policy" "platform" {
  for_each   = aws_ecr_repository.platform
  repository = each.value.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep the last 10 images"
      selection    = { tagStatus = "any", countType = "imageCountMoreThan", countNumber = 10 }
      action       = { type = "expire" }
    }]
  })
}
