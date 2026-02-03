############################################
# Karpenter (Controller IRSA + Node Role)
############################################

# NOTE:
# Don't reuse local name "oidc_issuer_hostpath" because alb_irsa.tf already defines it.
# Terraform locals are module-wide -> duplicate names will fail.
locals {
  karpenter_oidc_issuer_hostpath = replace(data.aws_eks_cluster.cluster.identity[0].oidc[0].issuer, "https://", "")
}

# -------------------------
# 1) Karpenter controller IRSA role
# -------------------------
data "aws_iam_policy_document" "karpenter_controller_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.oidc.arn]
    }

    # Require BOTH sub + aud for tighter security
    condition {
      test     = "StringEquals"
      variable = "${local.karpenter_oidc_issuer_hostpath}:sub"
      values   = ["system:serviceaccount:karpenter:karpenter"]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.karpenter_oidc_issuer_hostpath}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "karpenter_controller" {
  name               = "${var.project_name}-karpenter-controller-irsa"
  assume_role_policy = data.aws_iam_policy_document.karpenter_controller_assume_role.json
}

# -------------------------
# 2) Karpenter node role (used by instances it launches)
# -------------------------
resource "aws_iam_role" "karpenter_node" {
  name = "${var.project_name}-karpenter-node-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "karpenter_node_policies" {
  for_each = toset([
    "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
    "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
    "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
    "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
  ])

  role       = aws_iam_role.karpenter_node.name
  policy_arn = each.value
}

# Optional:
# If you use EC2NodeClass.spec.role, Karpenter can create/associate instance profiles itself.
# Keeping this instance profile is still okay (and can be useful for your own debugging),
# but Karpenter may also create its own profile name internally.
resource "aws_iam_instance_profile" "karpenter_node" {
  name = "${var.project_name}-karpenter-instance-profile"
  role = aws_iam_role.karpenter_node.name
}

# -------------------------
# 3) Interruption queue (Spot interruptions, rebalance, etc.)
# -------------------------
resource "aws_sqs_queue" "karpenter_interruption" {
  name                      = "${var.project_name}-karpenter-interruptions"
  message_retention_seconds = 300
}

# EventBridge -> SQS policy (restricted to your account + event rules)
data "aws_caller_identity" "current" {}

data "aws_iam_policy_document" "karpenter_sqs_policy" {
  statement {
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["events.amazonaws.com"]
    }

    actions   = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.karpenter_interruption.arn]

    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
  }
}

resource "aws_sqs_queue_policy" "karpenter_interruption" {
  queue_url = aws_sqs_queue.karpenter_interruption.id
  policy    = data.aws_iam_policy_document.karpenter_sqs_policy.json
}

# Create EventBridge rules that forward events to SQS
resource "aws_cloudwatch_event_rule" "spot_interruption" {
  name = "${var.project_name}-karpenter-spot-interruption"

  event_pattern = jsonencode({
    source        = ["aws.ec2"]
    "detail-type" = ["EC2 Spot Instance Interruption Warning"]
  })
}

resource "aws_cloudwatch_event_target" "spot_interruption" {
  rule      = aws_cloudwatch_event_rule.spot_interruption.name
  target_id = "KarpenterInterruptionQueue"
  arn       = aws_sqs_queue.karpenter_interruption.arn
}

resource "aws_cloudwatch_event_rule" "rebalance" {
  name = "${var.project_name}-karpenter-rebalance"

  event_pattern = jsonencode({
    source        = ["aws.ec2"]
    "detail-type" = ["EC2 Instance Rebalance Recommendation"]
  })
}

resource "aws_cloudwatch_event_target" "rebalance" {
  rule      = aws_cloudwatch_event_rule.rebalance.name
  target_id = "KarpenterInterruptionQueue"
  arn       = aws_sqs_queue.karpenter_interruption.arn
}

resource "aws_cloudwatch_event_rule" "instance_state_change" {
  name = "${var.project_name}-karpenter-instance-state-change"

  event_pattern = jsonencode({
    source        = ["aws.ec2"]
    "detail-type" = ["EC2 Instance State-change Notification"]
  })
}

resource "aws_cloudwatch_event_target" "instance_state_change" {
  rule      = aws_cloudwatch_event_rule.instance_state_change.name
  target_id = "KarpenterInterruptionQueue"
  arn       = aws_sqs_queue.karpenter_interruption.arn
}

# -------------------------
# 4) Karpenter controller policy
# -------------------------
data "aws_iam_policy_document" "karpenter_controller_policy" {
  # Core EC2 + cluster discovery
  statement {
    effect = "Allow"
    actions = [
      "ec2:CreateFleet",
      "ec2:RunInstances",
      "ec2:CreateLaunchTemplate",
      "ec2:CreateLaunchTemplateVersion",
      "ec2:DeleteLaunchTemplate",
      "ec2:CreateTags",
      "ec2:DeleteTags",
      "ec2:TerminateInstances",
      "ec2:Describe*",
      "pricing:GetProducts",
      "ssm:GetParameter",
      "eks:DescribeCluster"
    ]
    resources = ["*"]
  }

  # IMPORTANT: Karpenter often manages Instance Profiles internally (even if you pre-create one)
  statement {
    effect = "Allow"
    actions = [
      "iam:CreateInstanceProfile",
      "iam:DeleteInstanceProfile",
      "iam:GetInstanceProfile",
      "iam:AddRoleToInstanceProfile",
      "iam:RemoveRoleFromInstanceProfile",
      "iam:TagInstanceProfile",
      "iam:UntagInstanceProfile"
    ]
    resources = ["*"]
  }

  # Allow passing the node role to instances Karpenter launches
  statement {
    effect    = "Allow"
    actions   = ["iam:PassRole"]
    resources = [aws_iam_role.karpenter_node.arn]
  }

  # Interruption queue access (controller reads messages)
  statement {
    effect = "Allow"
    actions = [
      "sqs:DeleteMessage",
      "sqs:GetQueueAttributes",
      "sqs:GetQueueUrl",
      "sqs:ReceiveMessage"
    ]
    resources = [aws_sqs_queue.karpenter_interruption.arn]
  }
}

resource "aws_iam_policy" "karpenter_controller" {
  name        = "${var.project_name}-karpenter-controller-policy"
  description = "Permissions for Karpenter controller"
  policy      = data.aws_iam_policy_document.karpenter_controller_policy.json
}

resource "aws_iam_role_policy_attachment" "karpenter_controller_attach" {
  role       = aws_iam_role.karpenter_controller.name
  policy_arn = aws_iam_policy.karpenter_controller.arn
}
