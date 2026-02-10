resource "aws_route53_zone" "primary" {
  name = var.root_domain

  tags = {
    Name = "${var.project_name}-hosted-zone"
  }
}
