# fintech-platform-infra

**Fintech Platform – Cloud Infrastructure, CI/CD & GitOps on AWS EKS**

This repository contains the production-grade **cloud infrastructure**, **CI pipelines**, and **GitOps delivery** setup for a containerized fintech microservices platform running on **AWS EKS**.

It covers everything from **infrastructure provisioning with Terraform**, to **container build & push with GitHub Actions**, to **continuous deployment with Argo CD**, and **production observability** using **Prometheus/Grafana** and **Loki/Promtail**.

---

## What Has Been Built

At the moment, this project provides:

- Fully provisioned AWS infrastructure using **Terraform**
- A managed Kubernetes (**EKS**) cluster
- Multiple fintech microservices running in Kubernetes
- CI pipelines for building and pushing container images
- GitOps-based deployments using **Argo CD**
- Production logging and monitoring
- Secure AWS access using **IAM Roles for Service Accounts (IRSA)**

This is not a demo setup — it is structured the way a real production fintech platform would be run.


Technology Stack

Infrastructure
	•	AWS
	•	Amazon EKS
	•	EC2
	•	EBS (via CSI Driver)
	•	S3
	•	IAM
	•	ECR
	•	RDS
	•	Terraform

Platform & Delivery
	•	Kubernetes
	•	Helm
	•	Argo CD (GitOps)
	•	GitHub Actions (CI)

Observability
	•	Prometheus
	•	Grafana
	•	Loki
	•	Promtail

Infrastructure Provisioning (Terraform)

Terraform is used to provision all AWS resources, including:
	•	VPC and networking
	•	Amazon EKS cluster and managed node groups
	•	IAM roles and policies
	•	Amazon ECR repositories
	•	Amazon RDS databases
	•	Amazon S3 bucket for Loki log storage

Deploy Infrastructure (Production)

cd terraform/environments/prod

terraform init
terraform validate
terraform plan -out prod.tfplan
terraform apply prod.tfplan


⸻

Kubernetes Platform (EKS)
	•	Runs on Amazon EKS
	•	Uses managed node groups
	•	Persistent storage provided by AWS EBS CSI driver
	•	Default StorageClass: gp2-csi

⸻

Microservices

The platform currently runs the following services:
	•	API Gateway
	•	Authentication Service
	•	User Service
	•	Payments Service

Each service:
	•	Runs as a Kubernetes Deployment
	•	Uses container images stored in Amazon ECR
	•	Is deployed via Argo CD using GitOps principles

⸻

CI – GitHub Actions

Each microservice includes a CI pipeline that:
	1.	Builds a Docker image
	2.	Authenticates to AWS using OIDC
	3.	Pushes the image to Amazon ECR

No long-lived AWS credentials are used.

⸻

CD – GitOps with Argo CD
	•	Argo CD continuously watches the fintech-gitops directory
	•	Any change to manifests or Helm values is automatically reconciled
	•	Development and Production environments are managed independently

Check Applications

kubectl -n argocd get applications

Manual Sync (If Required)

argocd app sync loki-prod


⸻

Observability

Monitoring (Prometheus & Grafana)
	•	Installed using kube-prometheus-stack
	•	Collects:
	•	Cluster metrics
	•	Node metrics
	•	Application metrics
	•	Grafana included for dashboards and visualization

Logging (Loki)
	•	Loki runs in SingleBinary mode
	•	Logs are stored in Amazon S3
	•	Uses IRSA for secure AWS access
	•	Persistent volume backed by EBS

Log Shipping (Promtail)
	•	Runs as a DaemonSet
	•	Collects logs from all Kubernetes nodes
	•	Pushes logs to Loki

⸻

Security Model
	•	IAM Roles for Service Accounts (IRSA) used throughout
	•	No static AWS credentials inside Kubernetes
	•	Separate IAM roles for:
	•	Loki → S3 access
	•	GitHub Actions → ECR access
	•	Least-privilege IAM policies applied

⸻

Environments

Development
	•	Lightweight workloads
	•	Minimal observability

Production
	•	Full monitoring and logging
	•	Persistent storage
	•	S3-backed log retention

⸻

Status

This repository represents an actively evolving production-grade fintech platform. Additional services, scaling strategies, and security enhancements will be added incrementally.