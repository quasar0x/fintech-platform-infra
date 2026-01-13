variable "github_org" {
  type        = string
  description = "GitHub org/user name, e.g. quasar0x"
}

variable "github_repo" {
  type        = string
  description = "GitHub repo name, e.g. fintech-platform-infra"
}

variable "role_name" {
  type        = string
  description = "IAM role name for GitHub Actions"
  default     = "github-actions-ecr-push"
}

variable "aws_account_id" {
  type        = string
  description = "AWS Account ID"
}

variable "aws_region" {
  type        = string
  description = "AWS Region"
  default     = "us-east-1"
}

variable "ecr_repository_names" {
  type        = list(string)
  description = "ECR repositories this role can push to"
}
