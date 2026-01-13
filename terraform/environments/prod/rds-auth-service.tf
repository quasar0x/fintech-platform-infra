
resource "random_password" "auth_db_password" {
  length  = 24
  special = true

  keepers = {
    rotate = "2026-01-13"
  }
}

resource "aws_db_subnet_group" "auth" {
  name       = "${var.project_name}-auth-db-subnets"
  subnet_ids = module.vpc.private_subnets

  tags = {
    Name = "${var.project_name}-auth-db-subnets"
  }
}

data "aws_vpc" "this" {
  id = module.vpc.vpc_id
}

resource "aws_security_group" "auth_db" {
  name        = "${var.project_name}-auth-db-sg"
  description = "Allow Postgres from within VPC"
  vpc_id      = module.vpc.vpc_id

  ingress {
    description = "Postgres from VPC CIDR"
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = [data.aws_vpc.this.cidr_block]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${var.project_name}-auth-db-sg"
  }
}

resource "aws_db_instance" "auth" {
  identifier = "${var.project_name}-auth-db"

  engine              = "postgres"
  engine_version      = "16"
  instance_class      = "db.t3.micro"
  allocated_storage   = 20
  storage_type        = "gp3"
  multi_az            = false
  publicly_accessible = true
  deletion_protection = false
  skip_final_snapshot = true
  apply_immediately   = true

  db_name  = "auth"
  username = "authadmin"
  password = random_password.auth_db_password.result

  vpc_security_group_ids = [aws_security_group.auth_db.id]
  db_subnet_group_name   = aws_db_subnet_group.auth.name

  backup_retention_period = 7

  tags = {
    Name = "${var.project_name}-auth-db"
  }
}

output "auth_db_endpoint" {
  value = aws_db_instance.auth.address
}

output "auth_db_port" {
  value = aws_db_instance.auth.port
}

output "auth_db_name" {
  value = aws_db_instance.auth.db_name
}

output "auth_db_user" {
  value = aws_db_instance.auth.username
}

output "auth_db_password" {
  value     = random_password.auth_db_password.result
  sensitive = true
}
