resource "aws_ecs_cluster" "main" {
  name = "foyr"

  setting {
    name  = "containerInsights"
    value = "disabled"
  }
}

resource "aws_ecs_cluster_capacity_providers" "main" {
  cluster_name       = aws_ecs_cluster.main.name
  capacity_providers = ["FARGATE", "FARGATE_SPOT"]

  default_capacity_provider_strategy {
    capacity_provider = "FARGATE"
    weight            = 1
  }
}

# Private phone book: deployments register as d-<id>.apps.foyr.internal.
resource "aws_service_discovery_private_dns_namespace" "apps" {
  name = "apps.foyr.internal"
  vpc  = aws_vpc.main.id
}

# Customer app tasks: no inbound yet (authproxy is added in step 8), outbound through fck-nat.
resource "aws_security_group" "tenant" {
  name        = "foyr-tenant"
  description = "Customer app tasks"
  vpc_id      = aws_vpc.main.id
  tags        = { Name = "foyr-tenant" }
}

resource "aws_vpc_security_group_egress_rule" "tenant_all" {
  security_group_id = aws_security_group.tenant.id
  description       = "Outbound to the internet through fck-nat"
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}
