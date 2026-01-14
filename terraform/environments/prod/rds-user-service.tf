resource "random_password" "user_db_password" {
  length  = 24
  special = true

  # Only allow special chars that RDS accepts (no / @ " or space)
  override_special = "!#$%&()*+-.:;<=>?[]^_{|}~"

  keepers = {
    rotate = "2026-01-14" # bump this to force a new password
  }
}

resource "aws_db_subnet_group" "user" {
  name       = "${var.project_name}-user-db-subnets"
  subnet_ids = module.vpc.private_subnets

  tags = {
    Name = "${var.project_name}-user-db-subnets"
  }
}

resource "aws_security_group" "user_db" {
  name        = "${var.project_name}-user-db-sg"
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
    Name = "${var.project_name}-user-db-sg"
  }
}

resource "aws_db_instance" "user" {
  identifier = "${var.project_name}-user-db"

  engine            = "postgres"
  engine_version    = "16"
  instance_class    = "db.t3.micro"
  allocated_storage = 20
  storage_type      = "gp3"
  multi_az          = false

  publicly_accessible = false # ✅ keep private
  deletion_protection = false
  skip_final_snapshot = true
  apply_immediately   = true

  db_name  = "user_service"
  username = "useradmin"
  password = random_password.user_db_password.result

  vpc_security_group_ids = [aws_security_group.user_db.id]
  db_subnet_group_name   = aws_db_subnet_group.user.name

  backup_retention_period = 7

  tags = {
    Name = "${var.project_name}-user-db"
  }
}

output "user_db_endpoint" {
  value = aws_db_instance.user.address
}

output "user_db_port" {
  value = aws_db_instance.user.port
}

output "user_db_name" {
  value = aws_db_instance.user.db_name
}

output "user_db_user" {
  value = aws_db_instance.user.username
}

output "user_db_password" {
  value     = random_password.user_db_password.result
  sensitive = true
}
