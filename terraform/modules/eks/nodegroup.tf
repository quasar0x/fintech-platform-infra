resource "aws_eks_node_group" "default" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "${var.project_name}-default-node-group"
  node_role_arn   = aws_iam_role.eks_node.arn
  subnet_ids      = var.private_subnets

  scaling_config {
    desired_size = 2
    min_size     = 1
    max_size     = 4
  }

  instance_types = ["t3.medium"]
  capacity_type  = "ON_DEMAND"

  # ✅ Force a supported node OS family post-AL2 deprecation
  ami_type = "AL2023_x86_64_STANDARD"

  # ✅ keep nodes aligned with control plane version
  version = var.cluster_version

  depends_on = [
    aws_iam_role_policy_attachment.node_policies
  ]

  tags = {
    Name = "${var.project_name}-eks-node-group"
  }
}
