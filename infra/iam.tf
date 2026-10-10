data "aws_caller_identity" "me" {}

locals {
  account_id = data.aws_caller_identity.me.account_id
  # Execution role name => its SSM folder under /foyr/.
  exec_roles = {
    platform = "platform"
    tenant   = "apps"
  }
}

data "aws_iam_policy_document" "codebuild_trust" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["codebuild.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [local.account_id]
    }
  }
}

data "aws_iam_policy_document" "codebuild" {
  statement {
    sid       = "EcrLogin"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }

  statement {
    sid = "PushTenantImages"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:InitiateLayerUpload",
      "ecr:UploadLayerPart",
      "ecr:CompleteLayerUpload",
      "ecr:PutImage",
      "ecr:BatchGetImage",
      "ecr:GetDownloadUrlForLayer",
    ]
    resources = ["arn:aws:ecr:${var.region}:${local.account_id}:repository/apps/*"]
  }

  statement {
    sid       = "ReadBuildTokens"
    actions   = ["ssm:GetParameters"]
    resources = ["arn:aws:ssm:${var.region}:${local.account_id}:parameter/foyr/builds/*"]
  }

  statement {
    sid       = "BuildLogs"
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["${aws_cloudwatch_log_group.builds.arn}:*"]
  }
}

# Badge for builds: read its clone token, push to apps/* and write build logs, nothing else.
resource "aws_iam_role" "codebuild" {
  name               = "foyr-codebuild"
  assume_role_policy = data.aws_iam_policy_document.codebuild_trust.json
}

resource "aws_iam_role_policy" "codebuild" {
  name   = "build"
  role   = aws_iam_role.codebuild.id
  policy = data.aws_iam_policy_document.codebuild.json
}

data "aws_iam_policy_document" "ecs_tasks_trust" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [local.account_id]
    }
  }
}

# Used by ECS before the app starts (pull image, logs, secrets); never visible to app code.
resource "aws_iam_role" "exec" {
  for_each           = local.exec_roles
  name               = "foyr-exec-${each.key}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_trust.json
}

resource "aws_iam_role_policy_attachment" "exec_base" {
  for_each   = local.exec_roles
  role       = aws_iam_role.exec[each.key].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

data "aws_iam_policy_document" "exec_secrets" {
  for_each = local.exec_roles

  statement {
    actions   = ["ssm:GetParameters"]
    resources = ["arn:aws:ssm:${var.region}:${local.account_id}:parameter/foyr/${each.value}/*"]
  }
}

resource "aws_iam_role_policy" "exec_secrets" {
  for_each = local.exec_roles
  name     = "secrets"
  role     = aws_iam_role.exec[each.key].id
  policy   = data.aws_iam_policy_document.exec_secrets[each.key].json
}
