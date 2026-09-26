# Second wall: tenant subnets may only send replies (ephemeral ports) to platform subnets.
resource "aws_network_acl" "tenant" {
  vpc_id     = aws_vpc.main.id
  subnet_ids = aws_subnet.tenant[*].id
  tags       = { Name = "foyr-tenant" }

  ingress {
    rule_no    = 100
    action     = "allow"
    protocol   = "-1"
    cidr_block = "0.0.0.0/0"
    from_port  = 0
    to_port    = 0
  }

  dynamic "egress" {
    for_each = local.platform_cidrs
    content {
      rule_no    = 100 + egress.key
      action     = "allow"
      protocol   = "tcp"
      cidr_block = egress.value
      from_port  = 32768
      to_port    = 65535
    }
  }

  dynamic "egress" {
    for_each = local.platform_cidrs
    content {
      rule_no    = 110 + egress.key
      action     = "deny"
      protocol   = "-1"
      cidr_block = egress.value
      from_port  = 0
      to_port    = 0
    }
  }

  egress {
    rule_no    = 200
    action     = "allow"
    protocol   = "-1"
    cidr_block = "0.0.0.0/0"
    from_port  = 0
    to_port    = 0
  }
}
