resource "helm_release" "aws_load_balancer_controller" {
  name       = "aws-load-balancer-controller"
  namespace  = "kube-system"
  repository = "https://aws.github.io/eks-charts"
  chart      = "aws-load-balancer-controller"
  version    = "1.7.2"

  create_namespace = false

  values = [
    yamlencode({
      clusterName  = module.eks.cluster_name
      region       = var.region
      vpcId        = module.vpc.vpc_id
      replicaCount = 2
      ingressClass = "alb"

      serviceAccount = {
        create = true
        name   = "aws-load-balancer-controller"
        annotations = {
          "eks.amazonaws.com/role-arn" = module.eks.alb_controller_role_arn
        }
      }
    })
  ]

  depends_on = [
    module.eks
  ]
}
