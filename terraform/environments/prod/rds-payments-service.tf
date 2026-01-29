resource "random_password" "payments_db_password" {
  length           = 24
  special          = true
  override_special = "!#$%&()*+-.:;<=>?[]^_{|}~"

  keepers = {
    rotate = "2026-01-16" # bump when you want rotation.
  }
}

resource "aws_db_subnet_group" "payments" {
  name       = "${var.project_name}-payments-db-subnets"
  subnet_ids = module.vpc.private_subnets

  tags = {
    Name = "${var.project_name}-payments-db-subnets"
  }
}

resource "aws_security_group" "payments_db" {
  name        = "${var.project_name}-payments-db-sg"
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
    Name = "${var.project_name}-payments-db-sg"
  }
}

resource "aws_db_instance" "payments" {
  identifier = "${var.project_name}-payments-db"

  engine            = "postgres"
  engine_version    = "16"
  instance_class    = "db.t3.micro"
  allocated_storage = 20
  storage_type      = "gp3"
  multi_az          = false

  publicly_accessible = false
  deletion_protection = false
  skip_final_snapshot = true
  apply_immediately   = true

  db_name  = "payments_service"
  username = "paymentsadmin"
  password = random_password.payments_db_password.result

  vpc_security_group_ids = [aws_security_group.payments_db.id]
  db_subnet_group_name   = aws_db_subnet_group.payments.name

  backup_retention_period = 7

  tags = {
    Name = "${var.project_name}-payments-db"
  }
}

output "payments_db_endpoint" {
  value = aws_db_instance.payments.address
}

output "payments_db_port" {
  value = aws_db_instance.payments.port
}

output "payments_db_name" {
  value = aws_db_instance.payments.db_name
}

output "payments_db_user" {
  value = aws_db_instance.payments.username
}

output "payments_db_password" {
  value     = random_password.payments_db_password.result
  sensitive = true
}
