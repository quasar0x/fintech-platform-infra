############################################
# Loki IRSA (Prod) — reuse EKS cluster data
############################################

# IMPORTANT:
# Do NOT declare data "aws_eks_cluster" "this" here.
# It already exists in provider.tf, so we reuse it.

data "aws_iam_openid_connect_provider" "eks" {
  url = data.aws_eks_cluster.this.identity[0].oidc[0].issuer
}

# Loki bucket name (set this in variables.tf / tfvars)
variable "loki_bucket_name" {
  description = "S3 bucket name for Loki chunks/index"
  type        = string
}

# Namespace + ServiceAccount used by Loki
variable "loki_namespace" {
  description = "Namespace where Loki runs"
  type        = string
  default     = "monitoring-prod"
}

variable "loki_serviceaccount_name" {
  description = "Kubernetes ServiceAccount name used by Loki"
  type        = string
  default     = "loki"
}

locals {
  oidc_issuer_hostpath = replace(data.aws_eks_cluster.this.identity[0].oidc[0].issuer, "https://", "")
}

data "aws_iam_policy_document" "loki_irsa_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [data.aws_iam_openid_connect_provider.eks.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_issuer_hostpath}:sub"
      values   = ["system:serviceaccount:${var.loki_namespace}:${var.loki_serviceaccount_name}"]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_issuer_hostpath}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "loki_irsa" {
  name               = "loki-irsa-prod"
  assume_role_policy = data.aws_iam_policy_document.loki_irsa_assume_role.json
}

data "aws_iam_policy_document" "loki_s3_policy" {
  statement {
    effect = "Allow"
    actions = [
      "s3:ListBucket"
    ]
    resources = [
      "arn:aws:s3:::${var.loki_bucket_name}"
    ]
  }

  statement {
    effect = "Allow"
    actions = [
      "s3:GetObject",
      "s3:PutObject",
      "s3:DeleteObject",
      "s3:ListBucketMultipartUploads",
      "s3:AbortMultipartUpload",
      "s3:ListMultipartUploadParts"
    ]
    resources = [
      "arn:aws:s3:::${var.loki_bucket_name}/*"
    ]
  }
}

resource "aws_iam_policy" "loki_s3" {
  name   = "loki-s3-prod"
  policy = data.aws_iam_policy_document.loki_s3_policy.json
}

resource "aws_iam_role_policy_attachment" "loki_attach" {
  role       = aws_iam_role.loki_irsa.name
  policy_arn = aws_iam_policy.loki_s3.arn
}

output "loki_irsa_role_arn" {
  value       = aws_iam_role.loki_irsa.arn
  description = "Attach this role ARN to Loki ServiceAccount via eks.amazonaws.com/role-arn annotation"
}
