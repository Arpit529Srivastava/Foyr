output "vpc_id" {
  value = aws_vpc.main.id
}

output "public_subnet_ids" {
  value = aws_subnet.public[*].id
}

output "platform_subnet_ids" {
  value = aws_subnet.platform[*].id
}

output "tenant_subnet_ids" {
  value = aws_subnet.tenant[*].id
}

output "ecs_cluster_name" {
  value = aws_ecs_cluster.main.name
}

output "platform_repository_urls" {
  value = { for k, r in aws_ecr_repository.platform : k => r.repository_url }
}

output "codebuild_project_name" {
  value = aws_codebuild_project.build.name
}

output "apps_namespace_id" {
  value = aws_service_discovery_private_dns_namespace.apps.id
}

output "exec_role_arns" {
  value = { for k, r in aws_iam_role.exec : k => r.arn }
}

output "alerts_topic_arn" {
  value = aws_sns_topic.alerts.arn
}

output "tenant_security_group_id" {
  value = aws_security_group.tenant.id
}

output "apps_log_group_name" {
  value = aws_cloudwatch_log_group.apps.name
}

output "apps_name_servers" {
  description = "Add these as NS records for apps at GoDaddy (ADR-031)."
  value       = aws_route53_zone.apps.name_servers
}

output "apps_load_balancer_dns" {
  value = aws_lb.apps.dns_name
}

output "github_deploy_role_arn" {
  value = aws_iam_role.github_deploy.arn
}
