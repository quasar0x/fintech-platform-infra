# Shared node security group used by Karpenter-provisioned nodes (and optionally other nodes).
# Tagged for Karpenter discovery.
resource "aws_security_group" "nodes" {
  name        = "${var.project_name}-eks-nodes-sg"
  description = "Shared node security group for EKS + Karpenter nodes"
  vpc_id      = var.vpc_id

  tags = {
    Name                     = "${var.project_name}-eks-nodes-sg"
    "karpenter.sh/discovery" = var.project_name
  }
}

# Allow nodes to talk to each other (pod-to-pod, kubelet, etc.)
resource "aws_security_group_rule" "nodes_ingress_self_all" {
  type              = "ingress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  security_group_id = aws_security_group.nodes.id
  self              = true
}

# Allow ALL egress (common default for nodes)
resource "aws_security_group_rule" "nodes_egress_all" {
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = aws_security_group.nodes.id
}

# ---------------------------------------------------------
# REQUIRED: allow EKS control-plane (cluster SG) -> nodes
# ---------------------------------------------------------

# API server / webhooks / admission controllers talking to pods on nodes (TLS)
resource "aws_security_group_rule" "nodes_ingress_from_cluster_443" {
  type                     = "ingress"
  from_port                = 443
  to_port                  = 443
  protocol                 = "tcp"
  security_group_id        = aws_security_group.nodes.id
  source_security_group_id = aws_eks_cluster.this.vpc_config[0].cluster_security_group_id
  description              = "Allow EKS control plane to communicate with nodes on 443"
}

# Control plane -> kubelet on nodes
resource "aws_security_group_rule" "nodes_ingress_from_cluster_10250" {
  type                     = "ingress"
  from_port                = 10250
  to_port                  = 10250
  protocol                 = "tcp"
  security_group_id        = aws_security_group.nodes.id
  source_security_group_id = aws_eks_cluster.this.vpc_config[0].cluster_security_group_id
  description              = "Allow EKS control plane to communicate with kubelet on 10250"
}
