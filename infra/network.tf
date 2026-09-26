data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  azs            = slice(data.aws_availability_zones.available.names, 0, var.az_count)
  public_cidrs   = [for i in range(var.az_count) : cidrsubnet(var.vpc_cidr, 8, i)]
  platform_cidrs = [for i in range(var.az_count) : cidrsubnet(var.vpc_cidr, 4, i + 1)]
  tenant_cidrs   = [for i in range(var.az_count) : cidrsubnet(var.vpc_cidr, 2, i + 1)]
}

resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true
  tags                 = { Name = "foyr" }
}

# Empties the default security group so nothing in it can talk to anything.
resource "aws_default_security_group" "default" {
  vpc_id = aws_vpc.main.id
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "foyr" }
}

# Load balancer and fck-nat only.
resource "aws_subnet" "public" {
  count                   = var.az_count
  vpc_id                  = aws_vpc.main.id
  availability_zone       = local.azs[count.index]
  cidr_block              = local.public_cidrs[count.index]
  map_public_ip_on_launch = true
  tags                    = { Name = "foyr-public-${local.azs[count.index]}", tier = "public" }
}

# foyrd, authproxy, database, builds.
resource "aws_subnet" "platform" {
  count                   = var.az_count
  vpc_id                  = aws_vpc.main.id
  availability_zone       = local.azs[count.index]
  cidr_block              = local.platform_cidrs[count.index]
  map_public_ip_on_launch = false
  tags                    = { Name = "foyr-platform-${local.azs[count.index]}", tier = "platform" }
}

# Customer app containers.
resource "aws_subnet" "tenant" {
  count                   = var.az_count
  vpc_id                  = aws_vpc.main.id
  availability_zone       = local.azs[count.index]
  cidr_block              = local.tenant_cidrs[count.index]
  map_public_ip_on_launch = false
  tags                    = { Name = "foyr-tenant-${local.azs[count.index]}", tier = "tenant" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "foyr-public" }

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }
}

# Internet route is added by the fck-nat module.
resource "aws_route_table" "private" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "foyr-private" }
}

resource "aws_route_table_association" "public" {
  count          = var.az_count
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table_association" "platform" {
  count          = var.az_count
  subnet_id      = aws_subnet.platform[count.index].id
  route_table_id = aws_route_table.private.id
}

resource "aws_route_table_association" "tenant" {
  count          = var.az_count
  subnet_id      = aws_subnet.tenant[count.index].id
  route_table_id = aws_route_table.private.id
}

# Free private path to S3, so image layers skip fck-nat.
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.main.id
  service_name      = "com.amazonaws.${var.region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.private.id]
  tags              = { Name = "foyr-s3" }
}
