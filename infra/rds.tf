# Platform database: Postgres 17 on the smallest Graviton size, private, encrypted (ADR-032).
resource "aws_security_group" "rds" {
  name        = "foyr-rds"
  description = "Platform database"
  vpc_id      = aws_vpc.main.id
  tags        = { Name = "foyr-rds" }
}

resource "aws_vpc_security_group_ingress_rule" "rds_from_foyrd" {
  security_group_id            = aws_security_group.rds.id
  description                  = "Only from foyrd"
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
  referenced_security_group_id = aws_security_group.foyrd.id
}

resource "aws_db_subnet_group" "platform" {
  name       = "foyr-platform"
  subnet_ids = aws_subnet.platform[*].id
}

# URL-safe so it can sit inside FOYR_DATABASE_URL; lives only in state and SSM.
resource "random_password" "db" {
  length  = 40
  special = false
}

resource "aws_db_instance" "main" {
  identifier                 = "foyr"
  engine                     = "postgres"
  engine_version             = "17"
  instance_class             = "db.t4g.micro"
  allocated_storage          = 20
  max_allocated_storage      = 50
  storage_type               = "gp3"
  storage_encrypted          = true
  db_name                    = "foyr"
  username                   = "foyr"
  password                   = random_password.db.result
  db_subnet_group_name       = aws_db_subnet_group.platform.name
  vpc_security_group_ids     = [aws_security_group.rds.id]
  publicly_accessible        = false
  multi_az                   = false
  backup_retention_period    = 7
  copy_tags_to_snapshot      = true
  deletion_protection        = true
  skip_final_snapshot        = false
  final_snapshot_identifier  = "foyr-final"
  auto_minor_version_upgrade = true
  apply_immediately          = false
}

# foyrd reads this at start through its execution role (/foyr/platform/*).
resource "aws_ssm_parameter" "database_url" {
  name  = "/foyr/platform/database-url"
  type  = "SecureString"
  value = "postgres://${aws_db_instance.main.username}:${random_password.db.result}@${aws_db_instance.main.address}:5432/${aws_db_instance.main.db_name}?sslmode=require"
}
