# authproxy on Fargate (ARM) in the platform subnets, behind the load balancer (ADR-031).
locals {
  authproxy_port = 8081
}

resource "aws_security_group" "authproxy" {
  name        = "foyr-authproxy"
  description = "authproxy tasks"
  vpc_id      = aws_vpc.main.id
  tags        = { Name = "foyr-authproxy" }
}

resource "aws_vpc_security_group_ingress_rule" "authproxy_from_alb" {
  security_group_id            = aws_security_group.authproxy.id
  description                  = "Only from the load balancer"
  ip_protocol                  = "tcp"
  from_port                    = local.authproxy_port
  to_port                      = local.authproxy_port
  referenced_security_group_id = aws_security_group.alb.id
}

resource "aws_vpc_security_group_egress_rule" "authproxy_to_apps" {
  security_group_id            = aws_security_group.authproxy.id
  description                  = "To customer apps"
  ip_protocol                  = "tcp"
  from_port                    = 3000
  to_port                      = 3000
  referenced_security_group_id = aws_security_group.tenant.id
}

resource "aws_vpc_security_group_egress_rule" "authproxy_https" {
  security_group_id = aws_security_group.authproxy.id
  description       = "Image pulls and logs through fck-nat"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv4         = "0.0.0.0/0"
}

# Apps accept traffic only from authproxy, only on their port.
resource "aws_vpc_security_group_ingress_rule" "tenant_from_authproxy" {
  security_group_id            = aws_security_group.tenant.id
  description                  = "Only from authproxy"
  ip_protocol                  = "tcp"
  from_port                    = 3000
  to_port                      = 3000
  referenced_security_group_id = aws_security_group.authproxy.id
}

# The image pipeline registers new revisions; this is only the first one.
resource "aws_ecs_task_definition" "authproxy" {
  family                   = "foyr-authproxy"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.exec["platform"].arn

  runtime_platform {
    cpu_architecture        = "ARM64"
    operating_system_family = "LINUX"
  }

  container_definitions = jsonencode([{
    name         = "authproxy"
    image        = "${aws_ecr_repository.platform["authproxy"].repository_url}:${var.authproxy_image_tag}"
    essential    = true
    portMappings = [{ containerPort = local.authproxy_port, protocol = "tcp" }]
    environment = [
      { name = "FOYR_ADDR", value = ":${local.authproxy_port}" },
      { name = "FOYR_APPS_DOMAIN", value = var.apps_domain },
      { name = "FOYR_STATIC_ROUTES", value = var.authproxy_static_routes },
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.platform.name
        awslogs-region        = var.region
        awslogs-stream-prefix = "authproxy"
      }
    }
  }])
}

resource "aws_ecs_service" "authproxy" {
  name                              = "authproxy"
  cluster                           = aws_ecs_cluster.main.id
  task_definition                   = aws_ecs_task_definition.authproxy.arn
  desired_count                     = 1
  health_check_grace_period_seconds = 30

  capacity_provider_strategy {
    capacity_provider = "FARGATE"
    weight            = 1
  }

  network_configuration {
    subnets          = aws_subnet.platform[*].id
    security_groups  = [aws_security_group.authproxy.id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.authproxy.arn
    container_name   = "authproxy"
    container_port   = local.authproxy_port
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  depends_on = [aws_lb_listener.https]

  # The image pipeline owns which revision runs.
  lifecycle {
    ignore_changes = [task_definition]
  }
}
