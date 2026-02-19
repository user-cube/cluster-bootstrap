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

**Runtime:**
- `kubectl` configured with access to the target cluster
- `helm` (for local template testing)
- `sops` and `age` (for secrets encryption/decryption) **or** `git-crypt` (alternative encryption backend)
- SSH private key with read access to this repo

**Development (optional):**
- `go` 1.25+ (to build the CLI from source)
- `task` ([Task runner](https://taskfile.dev/))
- `pre-commit` ([pre-commit hooks](https://pre-commit.com/))

## Quick Start

### 1. Customize the template (first time only)

Replace the default `user-cube/cluster-bootstrap` with your organization and repository:

```bash
./cluster-bootstrap-cli/cluster-bootstrap-cli template customize --org mycompany --repo k8s-gitops
```

This updates Git URLs, GitHub badges, Go module paths, and documentation throughout the codebase. See [Template documentation](docs/cli/template.md) for details.

### 2. Install the CLI

**Option A: Install globally with `go install` (recommended)**
```bash
# From local source
go install ./cluster-bootstrap-cli

# Or from GitHub
go install github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli@latest

# Verify installation
cluster-bootstrap-cli --help
```

**Option B: Build locally**
```bash
task build
# Binary will be at: cluster-bootstrap-cli/cluster-bootstrap-cli
./cluster-bootstrap-cli/cluster-bootstrap-cli --help
```

**Option C: Use task helper**
```bash
task install
# Builds and installs to $(go env GOPATH)/bin
```

**Option D: Use Docker**
```bash
# Use the pre-built image from GHCR
docker run --rm -v $(pwd):/work -v ~/.kube:/root/.kube ghcr.io/user-cube/cluster-bootstrap-cli:latest bootstrap dev
```

### 3. Initialize secrets

```bash
./cluster-bootstrap-cli/cluster-bootstrap-cli init
```

### 4. Bootstrap the cluster

```bash
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev
```

This will:

1. Decrypt environment secrets (SOPS + age by default, or git-crypt)
2. Create the `argocd` namespace and SSH credentials secret
3. Install ArgoCD via Helm
4. Deploy the root **App of Apps** Application

> **💡 Idempotent by design**: The bootstrap command can be safely run multiple times. It automatically detects existing resources and updates them instead of failing. Perfect for configuration updates or GitOps workflows.

#### Bootstrap Reports

The bootstrap command generates comprehensive reports with timing metrics, resource operations, and health check results:

```bash
# Default: Human-readable summary report
./cluster-bootstrap-cli/cluster-bootstrap bootstrap dev

# JSON report for automation/metrics collection
./cluster-bootstrap-cli/cluster-bootstrap bootstrap dev --report-format json

# Save report to file
./cluster-bootstrap-cli/cluster-bootstrap bootstrap dev --report-output bootstrap-report.json
```

See [Bootstrap Reports documentation](docs/cli/bootstrap.md#bootstrap-reports) for details.

#### Using git-crypt instead of SOPS

```bash
./cluster-bootstrap-cli/cluster-bootstrap init --provider git-crypt
./cluster-bootstrap-cli/cluster-bootstrap bootstrap dev --encryption git-crypt
```

#### Repo content in a subdirectory

If your Kubernetes manifests live in a subdirectory (e.g. `k8s/`), you need to configure both the CLI and values file:

1. **Update `apps/values.yaml`** to set the base path:
```yaml
repo:
  basePath: "k8s"  # Set to your subdirectory name
```

2. **Run bootstrap** using either method:

From repository root:
```bash
./k8s/cli/cluster-bootstrap --base-dir ./k8s bootstrap dev \
  --app-path k8s/apps \
  --wait-for-health -v
```

Or from inside the subdirectory (both work):
```bash
cd k8s

# Relative path
./cluster-bootstrap-cli/cluster-bootstrap bootstrap dev --app-path apps --wait-for-health -v

# Or full path
./cluster-bootstrap-cli/cluster-bootstrap bootstrap dev --app-path k8s/apps --wait-for-health -v
```

**Key points:**
- The CLI **automatically detects** if you're in a Git subdirectory
- Works with both relative (`apps`) and full paths (`k8s/apps`)
- Strips prefixes intelligently for local validation
- `repo.basePath: "k8s"` in values.yaml ensures component paths include the subdirectory prefix
- Choose whichever feels most natural to you!

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
| `bootstrap <env>` | Full cluster bootstrap (decrypt secrets, install ArgoCD, deploy App of Apps). Generates comprehensive reports with timing metrics and resource operations. Fully idempotent. |
| `template customize` | Customize the template with your organization and repository (replaces placeholders in configs, docs, and code) |
| `doctor` | Run prerequisite checks for tooling and cluster access |
| `status <env>` | Show cluster status and component information |
| `validate <env>` | Validate local config, secrets, and optional cluster access |
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

Or use the CLI interactively: `./cluster-bootstrap-cli/cluster-bootstrap init myenv`

**git-crypt:** Secrets are stored as plaintext YAML (`secrets.<env>.yaml`) and encrypted transparently by git-crypt on commit:

```bash
git-crypt init
./cluster-bootstrap-cli/cluster-bootstrap init --provider git-crypt myenv
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
