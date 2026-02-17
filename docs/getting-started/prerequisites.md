# Prerequisites

Before bootstrapping a cluster, ensure the following tools are installed.

## Required Tools

| Tool | Purpose | Installation |
|------|---------|-------------|
| `kubectl` | Kubernetes CLI | [Install kubectl](https://kubernetes.io/docs/tasks/tools/) |
| `helm` | Helm package manager | [Install Helm](https://helm.sh/docs/intro/install/) |
| `sops` | Encrypted secrets management | [Install SOPS](https://github.com/getsops/sops) |
| `age` | Encryption tool (used by SOPS) | [Install age](https://github.com/FiloSottile/age) |
| `go` | To install/build the CLI tool | [Install Go](https://go.dev/doc/install) (1.25+) |

## Development Tools (optional)

Only needed if building from source or contributing:

| Tool | Purpose | Installation |
|------|---------|-------------|
| `task` | Task runner for development | [Install Task](https://taskfile.dev/installation/) |
| `pre-commit` | Git hooks for code quality | [Install pre-commit](https://pre-commit.com/) |

## Cluster Access

You need a running Kubernetes cluster with `kubectl` configured to access it. Any conformant cluster works — local (kind, minikube, k3s) or cloud-managed (EKS, GKE, AKS).

Verify access:

```bash
kubectl cluster-info
kubectl get nodes
```

## SSH Key

An SSH key pair with read access to the Git repository is required. ArgoCD uses this key to pull manifests from the repo.

```bash
ssh-keygen -t ed25519 -f repo-ssh-key.pem -N ""
```

Add the public key (`repo-ssh-key.pem.pub`) as a deploy key in your repository settings.

## Age Key

An age key pair is used by SOPS to encrypt environment-specific secrets.

```bash
age-keygen -o age-key.txt
```

The public key (printed to stdout) is configured in `.sops.yaml`. Keep `age-key.txt` safe — it is required for decrypting secrets during bootstrap.

## MkDocs (optional)

To preview this documentation site locally:

```bash
pip install mkdocs-material
mkdocs serve
```
