# GitHub Actions on main deploys platform images with short-lived credentials, no stored keys (ADR-031).
# The repo uses GitHub's immutable OIDC subject: owner and repo IDs, so a recreated repo with the same name gets nothing.
locals {
  github_repo = "Arpit529Srivastava@151747267/Foyr@1388194589"
}

# One per account and already shared with another project, so it is read, never owned here.
data "aws_iam_openid_connect_provider" "github" {
  url = "https://token.actions.githubusercontent.com"
}

data "aws_iam_policy_document" "github_trust" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [data.aws_iam_openid_connect_provider.github.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    # Only workflows running on main, never pull requests or other branches.
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:${local.github_repo}:ref:refs/heads/main"]
    }
  }
}

data "aws_iam_policy_document" "github_deploy" {
  statement {
    sid       = "EcrLogin"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }

  statement {
    sid = "PushPlatformImages"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:InitiateLayerUpload",
      "ecr:UploadLayerPart",
      "ecr:CompleteLayerUpload",
      "ecr:PutImage",
      "ecr:BatchGetImage",
      "ecr:DescribeImages",
    ]
    resources = [aws_ecr_repository.platform["authproxy"].arn, aws_ecr_repository.platform["foyrd"].arn]
  }

  # Task definitions can't be scoped by ARN for these two calls.
  statement {
    sid       = "TaskDefinitions"
    actions   = ["ecs:RegisterTaskDefinition", "ecs:DescribeTaskDefinition"]
    resources = ["*"]
  }

  statement {
    sid     = "DeployPlatform"
    actions = ["ecs:UpdateService", "ecs:DescribeServices"]
    resources = [
      for s in ["authproxy", "foyrd"] : "arn:aws:ecs:${var.region}:${local.account_id}:service/${aws_ecs_cluster.main.name}/${s}"
    ]
  }

  statement {
    sid       = "PassPlatformRoles"
    actions   = ["iam:PassRole"]
    resources = [aws_iam_role.exec["platform"].arn, aws_iam_role.foyrd.arn]

    condition {
      test     = "StringEquals"
      variable = "iam:PassedToService"
      values   = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "github_deploy" {
  name                 = "foyr-github-deploy"
  assume_role_policy   = data.aws_iam_policy_document.github_trust.json
  max_session_duration = 3600
}

resource "aws_iam_role_policy" "github_deploy" {
  name   = "deploy"
  role   = aws_iam_role.github_deploy.id
  policy = data.aws_iam_policy_document.github_deploy.json
}
