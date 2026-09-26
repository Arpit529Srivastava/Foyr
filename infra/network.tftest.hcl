mock_provider "aws" {
  mock_data "aws_availability_zones" {
    defaults = {
      names = ["ap-south-1a", "ap-south-1b", "ap-south-1c"]
    }
  }

  mock_data "aws_region" {
    defaults = {
      name   = "ap-south-1"
      region = "ap-south-1"
    }
  }

  mock_data "aws_ami" {
    defaults = {
      id = "ami-12345678"
    }
  }

  mock_resource "aws_iam_policy" {
    defaults = {
      arn = "arn:aws:iam::123456789012:policy/mock"
    }
  }

  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::123456789012:role/mock"
    }
  }

  mock_resource "aws_iam_instance_profile" {
    defaults = {
      arn = "arn:aws:iam::123456789012:instance-profile/mock"
    }
  }

  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}"
    }
  }
}

run "network_layout" {
  command = plan

  assert {
    condition     = length(aws_subnet.tenant) == 2 && aws_subnet.tenant[1].availability_zone == "ap-south-1b"
    error_message = "subnets must span exactly two zones"
  }

  assert {
    condition     = alltrue([for s in concat(aws_subnet.platform, aws_subnet.tenant) : !s.map_public_ip_on_launch])
    error_message = "private subnets must never assign public IPs"
  }

  assert {
    condition = length(distinct(concat(local.public_cidrs, local.platform_cidrs, local.tenant_cidrs))) == 6 && alltrue(flatten([
      for a in concat(local.public_cidrs, local.platform_cidrs, local.tenant_cidrs) : [
        for b in concat(local.public_cidrs, local.platform_cidrs, local.tenant_cidrs) :
        a == b || cidrhost(a, 0) != cidrhost(b, 0) && !(
          parseint(split(".", cidrhost(a, 0))[2], 10) >= parseint(split(".", cidrhost(b, 0))[2], 10) &&
          parseint(split(".", cidrhost(a, 0))[2], 10) <= parseint(split(".", cidrhost(b, -1))[2], 10)
        )
      ]
    ]))
    error_message = "subnet ranges must not overlap"
  }

  assert {
    condition     = length([for r in aws_route_table.private.route : r if r.gateway_id != null && r.gateway_id != ""]) == 0
    error_message = "private route table must not route to the internet gateway"
  }

  assert {
    condition = alltrue([
      for c in local.platform_cidrs :
      length([for e in aws_network_acl.tenant.egress : e if e.cidr_block == c && e.action == "deny" && e.protocol == "-1"]) == 1
    ])
    error_message = "tenant NACL must deny each platform range"
  }

  assert {
    condition     = aws_network_acl.tenant.egress != null && length([for e in aws_network_acl.tenant.egress : e if e.action == "allow" && e.from_port == 32768]) == 2
    error_message = "tenant NACL must allow replies to platform on ephemeral ports only"
  }
}
