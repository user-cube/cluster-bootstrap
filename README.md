# Cluster Bootstrap

[![CI](https://github.com/user-cube/cluster-bootstrap/actions/workflows/ci.yml/badge.svg)](https://github.com/user-cube/cluster-bootstrap/actions/workflows/ci.yml)
[![Release](https://github.com/user-cube/cluster-bootstrap/actions/workflows/release.yml/badge.svg)](https://github.com/user-cube/cluster-bootstrap/actions/workflows/release.yml)

GitOps repo for bootstrapping Kubernetes clusters with ArgoCD using the **App of Apps** pattern.

## Documentation

Full documentation is available at the [MkDocs site](docs/index.md). To preview locally:

```bash
pip install mkdocs-material
mkdocs serve
```

Online documentation available at [Cluster Boostrap Docs](https://user-cube.github.io/cluster-bootstrap/)

## Prerequisites

- `kubectl` configured with access to the target cluster
- `helm` (for local template testing)
- `sops` and `age` (for secrets encryption/decryption) **or** `git-crypt` (alternative encryption backend)
- `go` 1.25+ (to build the CLI)
- `task` ([Task runner](https://taskfile.dev/))
- `pre-commit` ([pre-commit hooks](https://pre-commit.com/))
- SSH private key with read access to this repo

## Quick Start

### 1. Build the CLI

```bash
task build
```

### 2. Initialize secrets (first time only)

```bash
./cli/cluster-bootstrap init
```

### 3. Bootstrap the cluster

```bash
./cli/cluster-bootstrap bootstrap dev
```

This will:

1. Decrypt environment secrets (SOPS + age by default, or git-crypt)
2. Create the `argocd` namespace and SSH credentials secret
3. Install ArgoCD via Helm
4. Deploy the root **App of Apps** Application

#### Using git-crypt instead of SOPS

```bash
./cli/cluster-bootstrap init --provider git-crypt
./cli/cluster-bootstrap bootstrap dev --encryption git-crypt
```

#### Repo content in a subdirectory

If your Kubernetes manifests live in a subdirectory (e.g. `k8s/`):

```bash
./cli/cluster-bootstrap --base-dir ./k8s bootstrap dev --app-path k8s/apps
```

`--base-dir` resolves local file paths (Chart.yaml, values, secrets). `--app-path` sets the `spec.source.path` in the ArgoCD Application CR.

### 4. Access ArgoCD UI

```bash
kubectl port-forward svc/argocd-server -n argocd 8080:443
```

Get the initial admin password:

```bash
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d
```

## Architecture

```
CLI bootstrap  →  ArgoCD + App of Apps (root Application)
                        ↓
                   apps/ (Helm chart with dynamic template)
                        ↓
              components/argocd/  (self-managed ArgoCD)
              components/xxx/    (other components)
```

ArgoCD manages itself — changes pushed to this repo are automatically synced.

The `apps/` chart uses a **single dynamic template** that iterates over a `components` map defined in `apps/values.yaml`. Adding a new component requires only a new entry in the values — no template files to create or copy.

## Components

| Component | Namespace | Sync Wave | Description |
|-----------|-----------|-----------|-------------|
| ArgoCD | `argocd` | 0 | Self-managed GitOps controller |
| Vault | `vault` | 1 | Secrets management |
| External Secrets | `external-secrets` | 1 | Syncs external secrets into Kubernetes |
| Prometheus Operator CRDs | `monitoring` | 2 | CRDs for the monitoring stack |
| ArgoCD Repo Secret | `argocd` | 2 | SSH credentials for repo access |
| Reloader | `reloader` | 2 | Restarts pods on ConfigMap/Secret changes |
| Kube Prometheus Stack | `monitoring` | 3 | Prometheus monitoring stack |
| Trivy Operator | `trivy-system` | 3 | Vulnerability scanning |

## CLI Commands

| Command | Description |
|---------|-------------|
| `bootstrap <env>` | Full cluster bootstrap (decrypt secrets, install ArgoCD, deploy App of Apps) |
| `init` | Interactive setup for encryption config and secrets files |
| `vault-token` | Store Vault root token as Kubernetes secret |
| `gitcrypt-key` | Store git-crypt symmetric key as Kubernetes secret |

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--base-dir` | `.` | Base directory for repo content (local file resolution) |
| `-v, --verbose` | `false` | Enable verbose output |

## Development

### Setup

```bash
pre-commit install
```

### Available tasks

Run `task --list` to see all available tasks. The most common ones:

```bash
task test         # Run Go tests with coverage
task lint         # Run golangci-lint
task helm-lint    # Lint Helm charts with templates
task fmt          # Format Go source files
task vet          # Run Go vet
task docs-serve   # Serve MkDocs documentation locally
```

### Secrets example

**SOPS (default):** `secrets.example.enc.yaml` contains the expected secrets structure. To create a new environment:

```bash
cp secrets.example.enc.yaml secrets.myenv.enc.yaml
sops --encrypt --in-place secrets.myenv.enc.yaml
```

Or use the CLI interactively: `./cli/cluster-bootstrap init myenv`

**git-crypt:** Secrets are stored as plaintext YAML (`secrets.<env>.yaml`) and encrypted transparently by git-crypt on commit:

```bash
git-crypt init
./cli/cluster-bootstrap init --provider git-crypt myenv
```

To use a custom `.sops.yaml` path, set `SOPS_CONFIG` in your `.env`:

```bash
SOPS_CONFIG=/path/to/custom/.sops.yaml
```

## Environments

| Environment | Values File | Description |
|-------------|-------------|-------------|
| dev | `apps/values/dev.yaml` | Local/development clusters, minimal resources |
| staging | `apps/values/staging.yaml` | Pre-production, moderate resources |
| prod | `apps/values/prod.yaml` | Production, HA configuration |

Environment files only need to set the `environment` key. Component defaults (namespace, sync wave, syncOptions, etc.) are defined in `apps/values.yaml`. To disable a component per environment:

```yaml
# apps/values/dev.yaml
environment: dev
components:
  trivy-operator:
    enabled: false
```
