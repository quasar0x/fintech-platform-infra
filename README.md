fintech-platform-infra

Fintech Platform – Cloud Infrastructure & GitOps

This repository contains the production-grade cloud infrastructure, CI/CD pipelines, and GitOps configuration for a containerized fintech microservices platform running on AWS EKS.

It covers the full lifecycle of a modern cloud-native platform:
	•	Infrastructure provisioning with Terraform
	•	Container build and push with GitHub Actions
	•	Continuous delivery using Argo CD (GitOps)
	•	Production-grade monitoring and logging with Prometheus, Grafana, Loki, and Promtail

This is not a demo setup. The repository is structured and operated the way a real-world fintech production platform would be.

⸻

What Has Been Built

At its current stage, this project provides:
	•	Fully provisioned AWS infrastructure using Terraform
	•	A managed Kubernetes cluster (Amazon EKS)
	•	Multiple fintech microservices running in Kubernetes
	•	CI pipelines for building and pushing container images to Amazon ECR
	•	GitOps-based deployments using Argo CD
	•	Centralized monitoring and logging
	•	Secure AWS access using IAM Roles for Service Accounts (IRSA)

⸻

Architecture Overview

High-Level Flow

GitHub (Application Code)
  └── GitHub Actions (CI)
        └── Build & Push Images → Amazon ECR

Infrastructure / GitOps Repository
  ├── Terraform → AWS (VPC, EKS, RDS, IAM, S3)
  └── Argo CD → Kubernetes (GitOps)

AWS Platform

AWS
 ├── VPC
 ├── EKS Cluster
 │    ├── Fintech Microservices (Dev & Prod)
 │    ├── Prometheus (Metrics)
 │    ├── Grafana (Dashboards)
 │    ├── Loki (Logs → S3)
 │    └── Promtail (Log Shipper)
 ├── ECR (Container Registry)
 ├── RDS (Databases)
 └── S3 (Loki Log Storage)


⸻

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

⸻

Repository Structure

fintech-platform-infra/
├── terraform/
│   ├── modules/
│   │   ├── vpc/
│   │   ├── eks/
│   │   ├── rds/
│   │   └── iam_github_actions_ecr/
│   └── environments/
│       ├── dev/
│       └── prod/
│
├── fintech-gitops/
│   ├── services/
│   │   ├── api-gateway/
│   │   ├── auth-service/
│   │   ├── payments-service/
│   │   └── user-service/
│   └── observability/
│       ├── monitoring/   # Prometheus / Grafana
│       └── logging/      # Loki + Promtail
│
└── README.md


⸻

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