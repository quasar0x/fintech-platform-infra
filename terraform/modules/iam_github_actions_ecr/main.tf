locals {
  repo_full = "repo:${var.github_org}/${var.github_repo}"
}

# 1) OIDC provider for GitHub (only one per account; safe to create if missing)
resource "aws_iam_openid_connect_provider" "github" {
  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = ["6938fd4d98bab03faadb97b34396831e3780aea1"]
}

# 2) Trust policy: allow only this repo + only develop/main branches
data "aws_iam_policy_document" "assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      values = [
        "${local.repo_full}:ref:refs/heads/develop",
        "${local.repo_full}:ref:refs/heads/main"
      ]
    }
  }
}

resource "aws_iam_role" "this" {
  name               = var.role_name
  assume_role_policy = data.aws_iam_policy_document.assume_role.json
}

# 3) Permissions: least-privilege ECR push to multiple repos
locals {
  ecr_repo_arns = [
    for name in var.ecr_repository_names :
    "arn:aws:ecr:${var.aws_region}:${var.aws_account_id}:repository/${name}"
  ]
}

data "aws_iam_policy_document" "ecr_push" {
  # Needed for docker login to ECR
  statement {
    effect    = "Allow"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }

  # Needed for build/push + CI tag checks
  statement {
    effect = "Allow"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:BatchGetImage",
      "ecr:CompleteLayerUpload",
      "ecr:DescribeImages",
      "ecr:DescribeRepositories",
      "ecr:GetDownloadUrlForLayer",
      "ecr:InitiateLayerUpload",
      "ecr:PutImage",
      "ecr:UploadLayerPart"
    ]
    resources = local.ecr_repo_arns
  }
}

resource "aws_iam_policy" "this" {
  name   = "${var.role_name}-policy"
  policy = data.aws_iam_policy_document.ecr_push.json
}

resource "aws_iam_role_policy_attachment" "attach" {
  role       = aws_iam_role.this.name
  policy_arn = aws_iam_policy.this.arn
}
