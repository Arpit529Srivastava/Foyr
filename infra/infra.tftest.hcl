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

  mock_resource "aws_lb" {
    defaults = {
      arn      = "arn:aws:elasticloadbalancing:ap-south-1:123456789012:loadbalancer/app/mock/1"
      dns_name = "mock.ap-south-1.elb.amazonaws.com"
      zone_id  = "ZP97RAFLXTNZK"
    }
  }

  mock_resource "aws_lb_target_group" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:ap-south-1:123456789012:targetgroup/mock/1"
    }
  }

  mock_resource "aws_acm_certificate" {
    defaults = {
      arn = "arn:aws:acm:ap-south-1:123456789012:certificate/mock"
    }
  }

  mock_data "aws_iam_openid_connect_provider" {
    defaults = {
      arn = "arn:aws:iam::123456789012:oidc-provider/token.actions.githubusercontent.com"
    }
  }

  mock_resource "aws_ecr_repository" {
    defaults = {
      arn            = "arn:aws:ecr:ap-south-1:123456789012:repository/mock"
      repository_url = "123456789012.dkr.ecr.ap-south-1.amazonaws.com/mock"
    }
  }

  mock_resource "aws_lb_listener" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:ap-south-1:123456789012:listener/app/mock/1/1"
    }
  }

  mock_resource "aws_service_discovery_service" {
    defaults = {
      arn = "arn:aws:servicediscovery:ap-south-1:123456789012:service/srv-mock"
    }
  }

  mock_resource "aws_db_instance" {
    defaults = {
      address = "foyr.abc.ap-south-1.rds.amazonaws.com"
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
    error_message = "tenant inbound rules live in separate rule resources only (ADR-029)"
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

run "front_door" {
  command = plan

  assert {
    condition     = aws_lb_listener.https.ssl_policy == "ELBSecurityPolicy-TLS13-1-2-2021-06" && aws_lb.apps.drop_invalid_header_fields
    error_message = "load balancer must use TLS 1.2+ and drop malformed headers"
  }

  assert {
    condition = alltrue([
      for a in aws_lb_listener.http.default_action : a.type == "redirect" && alltrue([for r in a.redirect : r.protocol == "HTTPS" && r.status_code == "HTTP_301"])
    ])
    error_message = "plain HTTP must only redirect to HTTPS"
  }

  assert {
    condition     = aws_acm_certificate.apps.domain_name == "*.${var.apps_domain}" && aws_route53_record.apps_wildcard.name == "*.${var.apps_domain}"
    error_message = "one wildcard certificate and record must cover every app"
  }

  assert {
    condition     = aws_vpc_security_group_ingress_rule.tenant_from_authproxy.from_port == 3000 && aws_vpc_security_group_ingress_rule.tenant_from_authproxy.to_port == 3000 && aws_vpc_security_group_ingress_rule.tenant_from_authproxy.cidr_ipv4 == null
    error_message = "apps may only be reached from authproxy, on port 3000"
  }

  assert {
    condition     = aws_vpc_security_group_ingress_rule.authproxy_from_alb.cidr_ipv4 == null && aws_vpc_security_group_egress_rule.alb_to_authproxy.cidr_ipv4 == null
    error_message = "authproxy may only be reached from the load balancer"
  }

  assert {
    condition = alltrue([for n in aws_ecs_service.authproxy.network_configuration : !n.assign_public_ip]) && alltrue([
      for c in aws_ecs_task_definition.authproxy.runtime_platform : c.cpu_architecture == "ARM64"
    ])
    error_message = "authproxy runs on ARM with no public IP (ADR-008)"
  }

  assert {
    condition = alltrue([
      for e in jsondecode(aws_ecs_task_definition.authproxy.container_definitions)[0].environment : !contains(["FOYR_INTERNAL_TOKEN", "FOYR_STATIC_ROUTES"], e.name)
    ]) && length([for x in jsondecode(aws_ecs_task_definition.authproxy.container_definitions)[0].secrets : x if x.name == "FOYR_INTERNAL_TOKEN"]) == 1
    error_message = "authproxy reads foyrd's routes, with the token from SSM, never as a plain value"
  }

  assert {
    condition = alltrue(flatten([
      for s in data.aws_iam_policy_document.github_trust.statement : [
        for c in s.condition : c.values == tolist(["repo:Arpit529Srivastava@151747267/Foyr@1388194589:ref:refs/heads/main"]) if c.variable == "token.actions.githubusercontent.com:sub"
      ]
    ])) && length(flatten([for s in data.aws_iam_policy_document.github_trust.statement : [for c in s.condition : c if c.variable == "token.actions.githubusercontent.com:sub"]])) == 1
    error_message = "only workflows on main may assume the deploy role"
  }

  assert {
    condition = alltrue([
      for s in data.aws_iam_policy_document.github_deploy.statement : !contains(s.actions, "iam:PassRole") || s.resources == toset([aws_iam_role.exec["platform"].arn, aws_iam_role.foyrd.arn])
    ])
    error_message = "the deploy role may only pass the platform execution role and foyrd's role"
  }
}

run "control_plane" {
  command = plan

  assert {
    condition     = !aws_db_instance.main.publicly_accessible && aws_db_instance.main.storage_encrypted && aws_db_instance.main.deletion_protection && aws_db_instance.main.backup_retention_period >= 7 && !aws_db_instance.main.skip_final_snapshot
    error_message = "the database must be private, encrypted, protected from deletion and backed up"
  }

  assert {
    condition     = aws_vpc_security_group_ingress_rule.rds_from_foyrd.cidr_ipv4 == null && aws_vpc_security_group_ingress_rule.rds_from_foyrd.from_port == 5432
    error_message = "only foyrd may reach the database"
  }

  assert {
    condition     = aws_vpc_security_group_ingress_rule.foyrd_from_authproxy.from_port == 9090 && aws_vpc_security_group_ingress_rule.foyrd_from_authproxy.cidr_ipv4 == null && aws_vpc_security_group_ingress_rule.foyrd_from_alb.from_port == 8080
    error_message = "the load balancer reaches only foyrd's public port; the internal port is for authproxy only (ADR-030)"
  }

  assert {
    condition     = aws_lb_target_group.foyrd.port == 8080 && aws_lb_target_group.authproxy.port == 8081
    error_message = "the load balancer must never forward to foyrd's internal listener"
  }

  assert {
    condition = alltrue([
      for e in jsondecode(aws_ecs_task_definition.foyrd.container_definitions)[0].environment : !strcontains(e.name, "SECRET") && !strcontains(e.name, "TOKEN") && !strcontains(e.name, "PRIVATE_KEY") && !strcontains(e.name, "FOUNDER_KEY") && e.name != "FOYR_DATABASE_URL"
    ]) && length(jsondecode(aws_ecs_task_definition.foyrd.container_definitions)[0].secrets) == 8
    error_message = "foyrd's secrets come from SSM, never plain environment values"
  }

  assert {
    condition = alltrue(flatten([
      for s in data.aws_iam_policy_document.foyrd.statement : [for a in s.actions : !strcontains(a, "*")]
      ])) && alltrue([
      for s in data.aws_iam_policy_document.foyrd.statement : s.resources == toset([aws_iam_role.exec["tenant"].arn]) if contains(s.actions, "iam:PassRole")
    ])
    error_message = "foyrd gets named actions only and may pass only the tenant execution role"
  }

  assert {
    condition = alltrue([
      for s in data.aws_iam_policy_document.foyrd.statement : alltrue([for r in s.resources : endswith(r, "/d-*")]) if contains(s.actions, "ecs:DeleteService")
    ])
    error_message = "foyrd may only create and delete app services (d-*), never platform services"
  }
}

# Magic-link login (ADR-035): keys and the Resend key come from SSM, the auth and app hosts reach foyrd.
run "login_before_dns" {
  command = plan

  assert {
    condition     = length(aws_acm_certificate_validation.platform_hosts) == 0 && length(aws_lb_listener_certificate.platform_hosts) == 0
    error_message = "the new certificate is not attached until its DNS records exist (platform_hosts_live)"
  }

  assert {
    condition     = flatten([for c in aws_lb_listener_rule.api.condition : [for h in c.host_header : tolist(h.values)]]) == ["api.foyr.online"]
    error_message = "before the certificate is live, only api. goes to foyrd"
  }
}

run "login_live" {
  command = plan

  variables {
    platform_hosts_live = true
  }

  assert {
    condition     = aws_acm_certificate.platform_hosts.domain_name == "auth.foyr.online" && aws_acm_certificate.platform_hosts.subject_alternative_names == toset(["app.foyr.online"]) && aws_acm_certificate.platform_hosts.validation_method == "DNS"
    error_message = "one certificate covers auth. and app."
  }

  assert {
    condition     = length(aws_lb_listener_certificate.platform_hosts) == 1
    error_message = "the certificate is attached to the HTTPS listener by SNI"
  }

  assert {
    condition = alltrue([for h in ["api.foyr.online", "auth.foyr.online", "app.foyr.online"] : contains(
      flatten([for c in aws_lb_listener_rule.api.condition : [for host in c.host_header : tolist(host.values)]]), h
    )])
    error_message = "api., auth. and app. all go to foyrd"
  }
}

run "login_secrets" {
  command = plan

  assert {
    condition = alltrue([
      for name, path in { FOYR_SESSION_KEY = "session-key", FOYR_RESEND_API_KEY = "resend-api-key" } :
      length([for x in jsondecode(aws_ecs_task_definition.foyrd.container_definitions)[0].secrets : x if x.name == name && endswith(x.valueFrom, "parameter/foyr/platform/${path}")]) == 1
    ])
    error_message = "foyrd reads the session key and the Resend key from SSM, not from plain values"
  }

  assert {
    condition = alltrue([
      for e in jsondecode(aws_ecs_task_definition.foyrd.container_definitions)[0].environment : e.name != "FOYR_SESSION_KEY" && e.name != "FOYR_RESEND_API_KEY"
    ]) && length([for e in jsondecode(aws_ecs_task_definition.foyrd.container_definitions)[0].environment : e if e.name == "FOYR_AUTH_URL" && e.value == "https://auth.foyr.online"]) == 1 && length([for e in jsondecode(aws_ecs_task_definition.foyrd.container_definitions)[0].environment : e if e.name == "FOYR_MAIL_FROM" && e.value == "Foyr <login@mail.foyr.online>"]) == 1
    error_message = "foyrd gets the auth URL and mail sender as plain settings and no key as plain text"
  }

  assert {
    condition = length([for x in jsondecode(aws_ecs_task_definition.authproxy.container_definitions)[0].secrets : x if x.name == "FOYR_SESSION_PUBLIC_KEY" && endswith(x.valueFrom, "parameter/foyr/platform/session-public-key")]) == 1 && alltrue([
      for x in jsondecode(aws_ecs_task_definition.authproxy.container_definitions)[0].secrets : !endswith(x.valueFrom, "/session-key") && !endswith(x.valueFrom, "/resend-api-key")
    ])
    error_message = "authproxy gets only the public key, never the signing key or the Resend key"
  }

  assert {
    condition     = length([for e in jsondecode(aws_ecs_task_definition.authproxy.container_definitions)[0].environment : e if e.name == "FOYR_AUTH_URL" && e.value == "https://auth.foyr.online"]) == 1 && length([for e in jsondecode(aws_ecs_task_definition.authproxy.container_definitions)[0].environment : e if e.name == "FOYR_FOYRD_URL" && e.value == "http://foyrd.platform.foyr.internal:9090"]) == 1
    error_message = "authproxy knows where to send visitors and where foyrd's private listener is"
  }
}

run "assertion_secrets" {
  command = plan

  assert {
    condition = length([for x in jsondecode(aws_ecs_task_definition.foyrd.container_definitions)[0].secrets : x if x.name == "FOYR_ASSERTION_PUBLIC_KEYS" && endswith(x.valueFrom, "parameter/foyr/platform/assertion-public-keys")]) == 1 && alltrue([
      for x in jsondecode(aws_ecs_task_definition.foyrd.container_definitions)[0].secrets : !endswith(x.valueFrom, "/assertion-key")
    ])
    error_message = "foyrd gets only the assertion public keys, never the signing key"
  }

  assert {
    condition     = length([for x in jsondecode(aws_ecs_task_definition.authproxy.container_definitions)[0].secrets : x if x.name == "FOYR_ASSERTION_KEY" && endswith(x.valueFrom, "parameter/foyr/platform/assertion-key")]) == 1
    error_message = "authproxy reads its assertion signing key from SSM"
  }

  assert {
    condition = alltrue(flatten([
      for td in [aws_ecs_task_definition.foyrd, aws_ecs_task_definition.authproxy] : [
        for e in jsondecode(td.container_definitions)[0].environment : !startswith(e.name, "FOYR_ASSERTION")
      ]
    ]))
    error_message = "no assertion key is a plain environment entry"
  }
}
