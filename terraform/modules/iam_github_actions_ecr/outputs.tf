output "role_arn" {
  description = "IAM Role ARN assumed by GitHub Actions via OIDC"
  value       = aws_iam_role.this.arn
}

output "role_name" {
  description = "IAM Role name"
  value       = aws_iam_role.this.name
}

output "oidc_provider_arn" {
  description = "GitHub Actions OIDC Provider ARN"
  value       = aws_iam_openid_connect_provider.github.arn
}
