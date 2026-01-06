variable "project_name" {
  description = "Project name prefix"
  type        = string
}

variable "vpc_id" {
  description = "VPC ID where EKS will be deployed"
  type        = string
}

variable "private_subnets" {
  description = "Private subnet IDs for EKS nodes"
  type        = list(string)
}


variable "cluster_version" {
  description = "Kubernetes version"
  type        = string
}
