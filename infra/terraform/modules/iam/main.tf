// IAM module: create roles and policies

data "aws_iam_policy_document" "eks_pod_assume_role" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.eks.arn]
    }
    condition {
      test     = "StringEquals"
      variable = "sts:Audience"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_openid_connect_provider" "eks" {
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = ["<thumbprint>"]
  url             = module.eks.cluster_oidc_issuer_url
}

resource "aws_iam_role" "pod_execution" {
  name               = "${var.environment}-eks-pod-execution-role"
  assume_role_policy = data.aws_iam_policy_document.eks_pod_assume_role.json
}

resource "aws_iam_role_policy_attachment" "pod_execution_attachment" {
  role       = aws_iam_role.pod_execution.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_POD_EXECUTION_ROLE_POLICY"
}
