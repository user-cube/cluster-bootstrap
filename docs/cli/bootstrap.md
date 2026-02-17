# bootstrap

```bash
./cli/cluster-bootstrap-cli bootstrap <environment>
```

Performs the full cluster bootstrap sequence.

```bash
./cli/cluster-bootstrap-cli bootstrap dev
```

## What it does

1. Loads secrets — decrypts via SOPS (default) or reads plaintext git-crypt files
2. Creates the `argocd` namespace
3. Creates the `repo-ssh-key` Secret with Git SSH credentials
4. Optionally creates `git-crypt-key` Secret (if `--gitcrypt-key-file` provided)
5. Installs ArgoCD via Helm (from `components/argocd/`)
6. Deploys the App of Apps root Application
7. Optionally waits for cluster components to be ready (if `--wait-for-health` provided)
8. Prints ArgoCD access instructions

## Idempotent Behavior

The bootstrap command is **fully idempotent** and can be safely run multiple times without causing errors or conflicts:

- **Namespace**: Verified and created only if it doesn't exist
- **Secrets**: Automatically updated if they already exist, created otherwise
- **ArgoCD Helm Release**: Upgraded if already installed, installed otherwise
- **App of Apps Application**: Updated with latest configuration if it exists, created otherwise

When running the command multiple times, you'll see clear feedback indicating whether each resource was **Created** or **Updated**:

```
✓ Created/verified namespace 'argocd'
  Created secret repo-ssh-key in argocd
  ✓ ArgoCD upgraded successfully
  ✓ App of Apps updated successfully
```

This makes bootstrap safe to re-run after configuration changes, secret updates, or as part of GitOps workflows.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--secrets-file` | auto | Path to secrets file. Auto-detected based on `--encryption`: `secrets.<env>.enc.yaml` (sops) or `secrets.<env>.yaml` (git-crypt). The file must exist. |
| `--encryption` | `sops` | Encryption backend: `sops` or `git-crypt` |
| `--dry-run` | `false` | Print manifests without applying |
| `--dry-run-output` | — | Write dry-run manifests to a file (JSON output) |
| `--skip-argocd-install` | `false` | Skip the Helm ArgoCD installation |
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig file |
| `--context` | current context | Kubeconfig context to use |
| `--age-key-file` | `SOPS_AGE_KEY_FILE` env | Path to age private key (SOPS only) |
| `--gitcrypt-key-file` | — | Path to git-crypt symmetric key file. When provided, stores the key as a `git-crypt-key` K8s Secret in the `argocd` namespace |
| `--app-path` | `apps` | Path inside the Git repo for the App of Apps source (used in the ArgoCD Application CR `spec.source.path`). If `apps` does not exist and no value is provided, the CLI auto-detects a matching chart (Chart.yaml + templates/application.yaml). |
| `--wait-for-health` | `false` | Wait for cluster components (ArgoCD, Vault, External Secrets) to be ready after bootstrap |
| `--health-timeout` | `180` | Timeout in seconds for health checks (default 180 = 3 minutes) |
| `--report-format` | `summary` | Report format: `summary`, `json`, or `none` |
| `--report-output` | — | Write JSON report to file |

## Examples

```bash
# SOPS (default)
./cli/cluster-bootstrap-cli bootstrap dev

# git-crypt
./cli/cluster-bootstrap-cli bootstrap dev --encryption git-crypt

# git-crypt with key stored in cluster + custom app path
./cli/cluster-bootstrap-cli bootstrap dev \
  --encryption git-crypt \
  --gitcrypt-key-file ./git-crypt-key \
  --app-path k8s/apps

# Dry run to a file
./cli/cluster-bootstrap-cli bootstrap dev --dry-run --dry-run-output /tmp/bootstrap.json

# Repo content in a subdirectory
# First, update apps/values.yaml to set repo.basePath: "k8s"

# Option 1: From repository root with --base-dir
./k8s/cli/cluster-bootstrap --base-dir ./k8s bootstrap dev \
  --app-path k8s/apps \
  --wait-for-health -v

# Option 2: From inside subdirectory (auto-detected)
cd k8s
./cli/cluster-bootstrap-cli bootstrap dev \
  --app-path apps \
  --wait-for-health -v

# Wait for components to be ready (with 5-minute timeout)
./cli/cluster-bootstrap-cli bootstrap dev --wait-for-health --health-timeout 300

# Wait for health with verbose output
./cli/cluster-bootstrap-cli bootstrap dev --wait-for-health -v
```

## Health Checks

When `--wait-for-health` is enabled, the CLI will verify that critical components are ready:

- **ArgoCD**: Checks if the `argocd-server` deployment has at least 1 ready replica
- **Vault**: Checks if the `vault` statefulset has ready replicas (if namespace exists)
- **External Secrets**: Checks if the `external-secrets` deployment has at least 1 ready replica (if namespace exists)

Each component is polled every 2 seconds with a default timeout of 180 seconds (3 minutes). If a component is not installed, it's marked as "NotInstalled" and doesn't fail the health check.

A detailed health status report is printed showing:
- Overall status (PASSED/FAILED)
- Individual component status (Ready, Timeout, NotInstalled, or Error)
- Duration for each component check
- Helpful messages for troubleshooting

## Bootstrap Reports

The bootstrap command generates a comprehensive report with detailed metrics about the bootstrap process, including stage timing, resource operations, and health check results.

### Report Formats

Three output formats are available via the `--report-format` flag:

- **`summary`** (default): Human-readable formatted output with tables and visual indicators
- **`json`**: JSON-formatted report to stdout for integration with automation tools
- **`none`**: Suppress report output

### Report Contents

The report includes:

- **Overall Status**: Success/failure with total duration
- **Stage Timing**: Duration for each bootstrap phase (Preflight Checks, Validation, Loading Secrets, K8s Resources, Installing ArgoCD, Deploying App of Apps, Health Checks)
- **Resource Operations**: Created vs Updated status for each resource (namespace, secrets, Helm releases, ArgoCD Applications)
- **Health Check Results**: Component health status when `--wait-for-health` is enabled
- **Configuration**: Environment, encryption method, and paths used

### Examples

```bash
# Default summary report
./cli/cluster-bootstrap-cli bootstrap dev

# JSON report to stdout (for piping to jq, logging systems, etc.)
./cli/cluster-bootstrap-cli bootstrap dev --report-format json

# Save JSON report to file for later analysis
./cli/cluster-bootstrap-cli bootstrap dev --report-output bootstrap-report.json

# JSON to both stdout and file
./cli/cluster-bootstrap-cli bootstrap dev --report-format json --report-output bootstrap-report.json

# Suppress report output (show only progress messages)
./cli/cluster-bootstrap-cli bootstrap dev --report-format none

# Full bootstrap with health checks and report
./cli/cluster-bootstrap-cli bootstrap dev --wait-for-health --report-output bootstrap-$(date +%Y%m%d-%H%M%S).json
```

### Sample Summary Report

```
Bootstrap Report
================

Status: ✅ Success
Environment: dev
Duration: 45.3s

Stages
------
Stage                     Duration  Status
Preflight Checks          2.1s      ✅
Validation                1.5s      ✅
Loading Secrets           3.2s      ✅
K8s Resources             5.8s      ✅
Installing ArgoCD         28.4s     ✅
Deploying App of Apps     2.3s      ✅
Health Checks             2.0s      ✅

Resources
---------
Resource           Operation
Namespace          Created
SSH Secret         Created
GitCrypt Secret    Updated
Helm Release       Upgraded
App of Apps        Created

Health Checks
-------------
Component          Status   Duration
ArgoCD             Ready    1.2s
Vault              Ready    0.5s
External Secrets   Ready    0.3s
```

### Sample JSON Report

```json
{
  "status": "success",
  "environment": "dev",
  "start_time": "2024-01-15T10:30:00Z",
  "end_time": "2024-01-15T10:30:45Z",
  "duration_seconds": 45.3,
  "stages": [
    {
      "name": "Preflight Checks",
      "duration_seconds": 2.1,
      "status": "completed"
    },
    {
      "name": "Validation",
      "duration_seconds": 1.5,
      "status": "completed"
    }
  ],
  "resources": {
    "namespace": {
      "name": "argocd",
      "created": true,
      "operation": "created"
    },
    "ssh_secret": {
      "name": "repo-ssh-secret",
      "created": true,
      "operation": "created"
    },
    "helm_release": {
      "name": "argocd",
      "created": false,
      "operation": "upgraded"
    }
  },
  "health": {
    "overall_status": "passed",
    "components": {
      "argocd": {
        "status": "ready",
        "duration_seconds": 1.2
      },
      "vault": {
        "status": "ready",
        "duration_seconds": 0.5
      }
    }
  },
  "config": {
    "environment": "dev",
    "encryption": "sops",
    "app_path": "apps"
  }
}
```
