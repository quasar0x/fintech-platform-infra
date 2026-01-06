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

