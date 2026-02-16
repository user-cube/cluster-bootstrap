# Cluster Bootstrap

A GitOps repository for bootstrapping Kubernetes clusters with ArgoCD using the **App of Apps** pattern.

## What is this?

This repo provides a fully automated, reproducible way to bootstrap a Kubernetes cluster with a complete platform stack. A single CLI command installs ArgoCD, which then self-manages and deploys all platform components through Helm charts.

## Key Features

- **App of Apps pattern** — ArgoCD manages itself and all components from a single root Application
- **Sync wave ordering** — components deploy in the correct dependency order
- **Multi-environment support** — dev, staging, and prod configurations with cascading values
- **Secrets management** — SOPS + age encryption with Vault and External Secrets Operator integration
- **CLI tool** — Go-based CLI automates the entire bootstrap process

## Components

| Component | Namespace | Sync Wave | Purpose |
|-----------|-----------|-----------|---------|
| ArgoCD | `argocd` | 0 | GitOps controller (self-managed) |
| Vault | `vault` | 1 | Secrets engine |
| External Secrets | `external-secrets` | 1 | Syncs secrets from Vault to Kubernetes |
| Prometheus Operator CRDs | `monitoring` | 2 | CRDs for monitoring stack |
| ArgoCD Repo Secret | `argocd` | 2 | Git repository credentials via ExternalSecret |
| Reloader | `reloader` | 2 | Restarts workloads on config changes |
| Kube Prometheus Stack | `monitoring` | 3 | Prometheus monitoring |
| Trivy Operator | `trivy-system` | 3 | Vulnerability scanning |

## Quick Links

- [Prerequisites](getting-started/prerequisites.md) — what you need before starting
- [Quick Start](getting-started/quick-start.md) — bootstrap a cluster in minutes
- [Troubleshooting](guides/troubleshooting.md) — common issues and solutions
- [Architecture](architecture/overview.md) — how the App of Apps pattern works
- [Adding a Component](guides/adding-a-component.md) — extend the stack with new components
- [Secrets Management](guides/secrets-management.md) — how encrypted secrets flow through the system
- [CLI Reference](cli/index.md) — available commands and usage
