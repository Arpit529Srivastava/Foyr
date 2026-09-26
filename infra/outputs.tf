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
