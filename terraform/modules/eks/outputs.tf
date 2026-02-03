output "cluster_name" {
  description = "EKS cluster name"
  value       = aws_eks_cluster.this.name
}

output "cluster_endpoint" {
  description = "EKS API server endpoint"
  value       = aws_eks_cluster.this.endpoint
}

output "oidc_provider_arn" {
  description = "OIDC provider ARN for IRSA"
  value       = aws_iam_openid_connect_provider.oidc.arn
}

output "alb_controller_role_arn" {
  value = aws_iam_role.alb_controller.arn
}

output "node_security_group_id" {
  description = "Security group id to use for worker nodes / Karpenter discovery"
  value       = aws_security_group.nodes.id
}

output "karpenter_controller_role_arn" {
  description = "IRSA role ARN for Karpenter controller"
  value       = aws_iam_role.karpenter_controller.arn
}

output "karpenter_node_role_name" {
  description = "IAM role name for Karpenter nodes"
  value       = aws_iam_role.karpenter_node.name
}

output "karpenter_interruption_queue_name" {
  description = "SQS queue name used by Karpenter for interruption handling"
  value       = aws_sqs_queue.karpenter_interruption.name
}

output "karpenter_instance_profile_name" {
  description = "Instance profile name for Karpenter nodes"
  value       = aws_iam_instance_profile.karpenter_node.name
}
