# Plans against a fake AWS; no credentials needed.
mock_provider "aws" {
  mock_data "aws_availability_zones" {
    defaults = {
      names = ["ap-south-1a", "ap-south-1b", "ap-south-1c"]
    }
  }

  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "123456789012"
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

  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}"
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

  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:ap-south-1:123456789012:log-group:mock"
    }
  }

  mock_resource "aws_sns_topic" {
    defaults = {
      arn = "arn:aws:sns:ap-south-1:123456789012:mock"
    }
  }
}

variables {
  alert_email = "alerts@example.com"
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

run "build_and_run" {
  command = plan

  assert {
    condition     = alltrue([for r in aws_ecr_repository.platform : r.image_tag_mutability == "IMMUTABLE" && alltrue([for c in r.image_scanning_configuration : c.scan_on_push])])
    error_message = "platform repositories must be immutable and scan on push"
  }

  assert {
    condition     = length(aws_codebuild_project.build.vpc_config) == 0
    error_message = "CodeBuild must run outside the VPC (ADR-020)"
  }

  assert {
    condition     = alltrue([for e in aws_codebuild_project.build.environment : strcontains(e.image, "x86_64") && e.compute_type == "BUILD_GENERAL1_MEDIUM"])
    error_message = "builds must be x86_64 on MEDIUM (ADR-020, ADR-008)"
  }

  assert {
    condition = alltrue(flatten([
      for s in data.aws_iam_policy_document.codebuild.statement : [
        for r in s.resources : s.sid == "EcrLogin" || endswith(r, ":repository/apps/*") || startswith(r, "arn:aws:logs:") || endswith(r, ":parameter/foyr/builds/*")
      ]
    ]))
    error_message = "CodeBuild may only push to apps/* repositories and read /foyr/builds/* tokens"
  }

  assert {
    condition = alltrue([
      for s in data.aws_iam_policy_document.codebuild.statement : s.actions == toset(["ssm:GetParameters"])
      if s.sid == "ReadBuildTokens"
    ])
    error_message = "CodeBuild may only read build tokens, never write SSM (ADR-027)"
  }

  assert {
    condition = alltrue([
      for s in aws_codebuild_project.build.source : strcontains(s.buildspec, filebase64("../templates/nextjs/Dockerfile")) && strcontains(s.buildspec, "http.extraHeader") && !strcontains(s.buildspec, "x-access-token@")
    ])
    error_message = "buildspec must embed the Next.js template and clone with the token in a header, not the URL"
  }

  assert {
    condition = alltrue(flatten([
      for k, d in data.aws_iam_policy_document.exec_secrets : [
        for s in d.statement : [for r in s.resources : endswith(r, ":parameter/foyr/${local.exec_roles[k]}/*")]
      ]
    ]))
    error_message = "each execution role may only read its own SSM folder"
  }

  assert {
    condition     = length(aws_security_group.tenant.ingress) == 0 && aws_vpc_security_group_egress_rule.tenant_all.cidr_ipv4 == "0.0.0.0/0"
    error_message = "tenant tasks accept no inbound traffic until authproxy exists (ADR-029)"
  }

  assert {
    condition     = aws_cloudwatch_log_group.builds.retention_in_days > 0 && aws_cloudwatch_log_group.platform.retention_in_days > 0 && aws_cloudwatch_log_group.apps.retention_in_days > 0
    error_message = "every log group must set retention (ADR-019)"
  }

  assert {
    condition     = contains(aws_ecs_cluster_capacity_providers.main.capacity_providers, "FARGATE") && contains(aws_ecs_cluster_capacity_providers.main.capacity_providers, "FARGATE_SPOT")
    error_message = "cluster must allow FARGATE and FARGATE_SPOT"
  }
}
