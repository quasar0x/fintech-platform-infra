terraform {
  backend "s3" {
    bucket       = "fintech-platform-tf-state"
    key          = "prod/terraform.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}

module "vpc" {
  source       = "../../modules/vpc"
  project_name = var.project_name
}

module "eks" {
  source          = "../../modules/eks"
  project_name    = var.project_name
  vpc_id          = module.vpc.vpc_id
  private_subnets = module.vpc.private_subnets
  cluster_version = var.cluster_version
}

module "github_actions_ecr" {
  source         = "../../modules/iam_github_actions_ecr"
  github_org     = "quasar0x"
  github_repo    = "fintech-platform-infra"
  aws_account_id = "461508717137"
  aws_region     = var.region
  role_name      = "github-actions-ecr-push"

  # allow pushing to both images
  ecr_repository_names = [
    "api-gateway",
    "auth-service",
    "user-service",
    "payments-service"
  ]
}


