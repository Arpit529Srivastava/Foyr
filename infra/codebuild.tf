# One shared build project; foyrd passes REPO_URL, COMMIT, IMAGE_URI and the token's SSM name per build.
resource "aws_codebuild_project" "build" {
  name           = "foyr-build"
  service_role   = aws_iam_role.codebuild.arn
  build_timeout  = 20
  queued_timeout = 30

  artifacts {
    type = "NO_ARTIFACTS"
  }

  source {
    type = "NO_SOURCE"
    buildspec = templatefile("${path.module}/buildspec.yml.tftpl", {
      nextjs_dockerfile   = filebase64("${path.module}/../templates/nextjs/Dockerfile")
      nextjs_dockerignore = filebase64("${path.module}/../templates/nextjs/dockerignore")
    })
  }

  environment {
    compute_type    = "BUILD_GENERAL1_MEDIUM"
    image           = "aws/codebuild/amazonlinux-x86_64-standard:5.0"
    type            = "LINUX_CONTAINER"
    privileged_mode = true
  }

  logs_config {
    cloudwatch_logs {
      group_name = aws_cloudwatch_log_group.builds.name
    }
  }
}
