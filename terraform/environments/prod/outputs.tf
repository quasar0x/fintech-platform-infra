output "github_actions_ecr_role_arn" {
  description = "IAM Role ARN for GitHub Actions ECR push"
  value       = module.github_actions_ecr.role_arn
}

output "github_actions_ecr_role_name" {
  description = "IAM Role name for GitHub Actions ECR push"
  value       = module.github_actions_ecr.role_name
}

output "github_actions_oidc_provider_arn" {
  description = "OIDC provider ARN used by GitHub Actions"
  value       = module.github_actions_ecr.oidc_provider_arn
}
