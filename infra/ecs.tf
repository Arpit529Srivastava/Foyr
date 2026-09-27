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
