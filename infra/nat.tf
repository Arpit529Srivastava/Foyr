# Outbound internet for private subnets on a t4g.nano; restarts itself if it dies.
locals {
  nat_name = "foyr-nat"
}

module "fck_nat" {
  source  = "RaJiska/fck-nat/aws"
  version = "1.6.1"

  name          = local.nat_name
  vpc_id        = aws_vpc.main.id
  subnet_id     = aws_subnet.public[0].id
  instance_type = "t4g.nano"
  ha_mode       = true

  update_route_tables = true
  route_tables_ids    = { private = aws_route_table.private.id }
}
