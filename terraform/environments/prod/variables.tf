variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "project_name" {
  description = "Project name prefix"
  type        = string
  default     = "fintech-platform"
}

variable "cluster_version" {
  description = "EKS Kubernetes minor version"
  type        = string
  default     = "1.34"
}

variable "root_domain" {
  description = "Root domain to create in Route53, e.g. fintech-platform.dev"
  type        = string
  default     = "fintech-platform.dev"
}
