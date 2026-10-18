# foyrd, the control plane, on Fargate (ARM) in the platform subnets (ADR-032).
locals {
  foyrd_port          = 8080
  foyrd_internal_port = 9090
  # Created by scripts/platform-secrets.sh from ~/.config/foyr, never by tofu, so they stay out of state.
  foyrd_secrets = {
    FOYR_GITHUB_WEBHOOK_SECRET = "github-webhook-secret"
    FOYR_GITHUB_PRIVATE_KEY    = "github-private-key"
    FOYR_INTERNAL_TOKEN        = "internal-token"
  }
}

resource "aws_security_group" "foyrd" {
  name        = "foyr-foyrd"
  description = "foyrd tasks"
  vpc_id      = aws_vpc.main.id
  tags        = { Name = "foyr-foyrd" }
}

resource "aws_vpc_security_group_ingress_rule" "foyrd_from_alb" {
  security_group_id            = aws_security_group.foyrd.id
  description                  = "Webhooks from the load balancer"
  ip_protocol                  = "tcp"
  from_port                    = local.foyrd_port
  to_port                      = local.foyrd_port
  referenced_security_group_id = aws_security_group.alb.id
}

resource "aws_vpc_security_group_ingress_rule" "foyrd_from_authproxy" {
  security_group_id            = aws_security_group.foyrd.id
  description                  = "Routes for authproxy, private listener only"
  ip_protocol                  = "tcp"
  from_port                    = local.foyrd_internal_port
  to_port                      = local.foyrd_internal_port
  referenced_security_group_id = aws_security_group.authproxy.id
}

resource "aws_vpc_security_group_egress_rule" "foyrd_https" {
  security_group_id = aws_security_group.foyrd.id
  description       = "AWS and GitHub APIs through fck-nat"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_vpc_security_group_egress_rule" "foyrd_to_rds" {
  security_group_id            = aws_security_group.foyrd.id
  description                  = "Platform database"
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
  referenced_security_group_id = aws_security_group.rds.id
}

resource "aws_vpc_security_group_egress_rule" "alb_to_foyrd" {
  security_group_id            = aws_security_group.alb.id
  description                  = "Webhooks to foyrd"
  ip_protocol                  = "tcp"
  from_port                    = local.foyrd_port
  to_port                      = local.foyrd_port
  referenced_security_group_id = aws_security_group.foyrd.id
}

resource "aws_vpc_security_group_egress_rule" "authproxy_to_foyrd" {
  security_group_id            = aws_security_group.authproxy.id
  description                  = "Routes from foyrd"
  ip_protocol                  = "tcp"
  from_port                    = local.foyrd_internal_port
  to_port                      = local.foyrd_internal_port
  referenced_security_group_id = aws_security_group.foyrd.id
}

# authproxy finds foyrd at foyrd.platform.foyr.internal.
resource "aws_service_discovery_private_dns_namespace" "platform" {
  name = "platform.foyr.internal"
  vpc  = aws_vpc.main.id
}

resource "aws_service_discovery_service" "foyrd" {
  name = "foyrd"

  dns_config {
    namespace_id   = aws_service_discovery_private_dns_namespace.platform.id
    routing_policy = "MULTIVALUE"
    dns_records {
      type = "A"
      ttl  = 10
    }
  }

  health_check_custom_config {}
}

# What foyrd's code may do in AWS: run builds and deployments, nothing else (ADR-032).
data "aws_iam_policy_document" "foyrd" {
  statement {
    sid       = "Builds"
    actions   = ["codebuild:StartBuild", "codebuild:BatchGetBuilds"]
    resources = [aws_codebuild_project.build.arn]
  }

  statement {
    sid       = "AppRepositories"
    actions   = ["ecr:CreateRepository", "ecr:DescribeRepositories", "ecr:PutLifecyclePolicy", "ecr:TagResource"]
    resources = ["arn:aws:ecr:${var.region}:${local.account_id}:repository/apps/*"]
  }

  statement {
    sid       = "BuildTokens"
    actions   = ["ssm:PutParameter", "ssm:DeleteParameter"]
    resources = ["arn:aws:ssm:${var.region}:${local.account_id}:parameter/foyr/builds/*"]
  }

  statement {
    sid       = "ReadLogs"
    actions   = ["logs:GetLogEvents"]
    resources = ["${aws_cloudwatch_log_group.builds.arn}:*", "${aws_cloudwatch_log_group.apps.arn}:*"]
  }

  # These ECS calls can't be scoped by ARN.
  statement {
    sid       = "TaskDefinitions"
    actions   = ["ecs:RegisterTaskDefinition", "ecs:ListTasks"]
    resources = ["*"]
  }

  statement {
    sid       = "AppServices"
    actions   = ["ecs:CreateService", "ecs:DeleteService", "ecs:DescribeServices"]
    resources = ["arn:aws:ecs:${var.region}:${local.account_id}:service/${aws_ecs_cluster.main.name}/d-*"]
  }

  statement {
    sid       = "AppTasks"
    actions   = ["ecs:DescribeTasks"]
    resources = ["arn:aws:ecs:${var.region}:${local.account_id}:task/${aws_ecs_cluster.main.name}/*"]
  }

  statement {
    sid       = "TagOnCreate"
    actions   = ["ecs:TagResource"]
    resources = ["*"]

    condition {
      test     = "StringEquals"
      variable = "ecs:CreateAction"
      values   = ["CreateService", "RegisterTaskDefinition"]
    }
  }

  statement {
    sid       = "AppNames"
    actions   = ["servicediscovery:CreateService", "servicediscovery:GetService", "servicediscovery:ListServices", "servicediscovery:DeleteService"]
    resources = ["*"]
  }

  statement {
    sid       = "PassTenantExecRole"
    actions   = ["iam:PassRole"]
    resources = [aws_iam_role.exec["tenant"].arn]

    condition {
      test     = "StringEquals"
      variable = "iam:PassedToService"
      values   = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "foyrd" {
  name               = "foyr-foyrd"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_trust.json
}

resource "aws_iam_role_policy" "foyrd" {
  name   = "control-plane"
  role   = aws_iam_role.foyrd.id
  policy = data.aws_iam_policy_document.foyrd.json
}

# The image pipeline registers new revisions; this is only the first one.
resource "aws_ecs_task_definition" "foyrd" {
  family                   = "foyr-foyrd"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.exec["platform"].arn
  task_role_arn            = aws_iam_role.foyrd.arn

  runtime_platform {
    cpu_architecture        = "ARM64"
    operating_system_family = "LINUX"
  }

  container_definitions = jsonencode([{
    name      = "foyrd"
    image     = "${aws_ecr_repository.platform["foyrd"].repository_url}:${var.foyrd_image_tag}"
    essential = true
    portMappings = [
      { containerPort = local.foyrd_port, protocol = "tcp" },
      { containerPort = local.foyrd_internal_port, protocol = "tcp" },
    ]
    environment = [
      { name = "FOYR_ADDR", value = ":${local.foyrd_port}" },
      { name = "FOYR_INTERNAL_ADDR", value = ":${local.foyrd_internal_port}" },
      { name = "FOYR_GITHUB_APP_ID", value = var.github_app_id },
      { name = "FOYR_AWS_REGION", value = var.region },
      { name = "FOYR_WORKERS", value = "2" },
      { name = "FOYR_CODEBUILD_PROJECT", value = aws_codebuild_project.build.name },
      { name = "FOYR_ECS_CLUSTER", value = aws_ecs_cluster.main.name },
      { name = "FOYR_TENANT_SUBNETS", value = join(",", aws_subnet.tenant[*].id) },
      { name = "FOYR_TENANT_SECURITY_GROUP", value = aws_security_group.tenant.id },
      { name = "FOYR_TENANT_EXEC_ROLE_ARN", value = aws_iam_role.exec["tenant"].arn },
      { name = "FOYR_APPS_NAMESPACE_ID", value = aws_service_discovery_private_dns_namespace.apps.id },
      { name = "FOYR_APPS_LOG_GROUP", value = aws_cloudwatch_log_group.apps.name },
      { name = "FOYR_APPS_CAPACITY", value = "FARGATE_SPOT" },
    ]
    secrets = concat(
      [{ name = "FOYR_DATABASE_URL", valueFrom = aws_ssm_parameter.database_url.arn }],
      [for env, name in local.foyrd_secrets : { name = env, valueFrom = "arn:aws:ssm:${var.region}:${local.account_id}:parameter/foyr/platform/${name}" }],
    )
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.platform.name
        awslogs-region        = var.region
        awslogs-stream-prefix = "foyrd"
      }
    }
  }])
}

resource "aws_lb_target_group" "foyrd" {
  name                 = "foyr-foyrd"
  target_type          = "ip"
  protocol             = "HTTP"
  port                 = local.foyrd_port
  vpc_id               = aws_vpc.main.id
  deregistration_delay = 30

  health_check {
    path                = "/healthz"
    matcher             = "200"
    interval            = 15
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}

# api.foyr.online goes to foyrd; every other host stays with authproxy.
resource "aws_lb_listener_rule" "api" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 10

  condition {
    host_header {
      values = [var.api_domain]
    }
  }

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.foyrd.arn
  }
}

resource "aws_ecs_service" "foyrd" {
  name                              = "foyrd"
  cluster                           = aws_ecs_cluster.main.id
  task_definition                   = aws_ecs_task_definition.foyrd.arn
  desired_count                     = 1
  health_check_grace_period_seconds = 60

  capacity_provider_strategy {
    capacity_provider = "FARGATE"
    weight            = 1
  }

  network_configuration {
    subnets          = aws_subnet.platform[*].id
    security_groups  = [aws_security_group.foyrd.id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.foyrd.arn
    container_name   = "foyrd"
    container_port   = local.foyrd_port
  }

  service_registries {
    registry_arn = aws_service_discovery_service.foyrd.arn
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  depends_on = [aws_lb_listener_rule.api]

  # The image pipeline owns which revision runs.
  lifecycle {
    ignore_changes = [task_definition]
  }
}
