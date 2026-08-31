# bootstrap

```bash
cluster-bootstrap-cli bootstrap <environment>
```

Performs the full cluster bootstrap sequence.

```bash
cluster-bootstrap-cli bootstrap dev
```

## What it does

1. Loads secrets — decrypts via SOPS (default) or reads plaintext git-crypt files
2. Checks the target cluster for an existing App of Apps and stops unless `--force` is set
3. Announces the target Kubernetes context and counts down for 10 seconds so the run can be aborted
4. Creates the `argocd` namespace
5. Creates the `repo-ssh-key` Secret with Git SSH credentials
6. Optionally creates `git-crypt-key` when `--gitcrypt-key-file` is provided, or `sops-age-key` when `--store-sops-age-key` is set
7. When `--enable-cilium` is set, installs or upgrades Cilium and waits for Helm's workload readiness checks to pass
8. Installs ArgoCD via Helm (from `components/argocd/`)
9. Deploys the App of Apps root Application, enabling its repository-backed Cilium Application when requested
10. Optionally waits for cluster components to be ready (if `--wait-for-health` provided)
11. Prints ArgoCD access instructions

## Safeguards

Before anything is written to the cluster, bootstrap performs two checks.

### Existing App of Apps

An `app-of-apps` Application in the `argocd` namespace means the cluster has already been bootstrapped. Bootstrap stops and reports what it found, without changing the cluster:

```
  Existing App of Apps:
    Application:  app-of-apps (namespace argocd)
    Repository:   git@github.com:acme/repo.git
    Revision:     main
    Path:         apps
    Sync/Health:  Synced / Healthy

ERROR cluster already bootstrapped: App of Apps "app-of-apps" exists in namespace argocd on context kind-dev
  hint: inspect the existing installation with: cluster-bootstrap-cli info <environment>
  tip: re-run with --force to overwrite the existing App of Apps
```

Pass `--force` to re-run against an already bootstrapped cluster. The existing App of Apps is still reported, then overwritten with the current configuration.

### Target context countdown

Bootstrap prints the Kubernetes context it is about to modify and waits 10 seconds so a run against the wrong cluster can be aborted with `Ctrl+C`:

```
⚠  Bootstrap will modify the cluster on Kubernetes context: kind-dev
    Starting in  7s... press Ctrl+C to abort
```

The context shown is the resolved one: the `--context` override when given, otherwise the current context of the kubeconfig in use. The countdown is skipped with `--yes`, and automatically when stdout is not a terminal, so CI pipelines are not delayed.

## Idempotent Behavior

Bootstrap is idempotent, so re-running it with `--force` after configuration changes or secret updates converges the cluster instead of failing:

- **Namespace**: Verified and created only if it doesn't exist
- **Secrets**: Automatically updated if they already exist, created otherwise
- **ArgoCD Helm Release**: Upgraded if already installed, installed otherwise
- **Cilium Helm Release**: When enabled, upgraded if already installed and left untouched when the flag is omitted
- **Cilium Application**: Disabled in the chart by default and rendered by the App of Apps only when the flag is enabled
- **App of Apps Application**: Updated with latest configuration if it exists, created otherwise

When running the command multiple times, you'll see clear feedback indicating whether each resource was **Created** or **Updated**:

```
✓ Created/verified namespace 'argocd'
  Created secret repo-ssh-key in argocd
  ✓ ArgoCD upgraded successfully
  ✓ App of Apps updated successfully
```

Without `--force`, the first run is the only one that reaches these steps — the safeguard above stops later runs.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--secrets-file` | auto | Path to secrets file. Auto-detected based on `--encryption`: `secrets.<env>.enc.yaml` (sops) or `secrets.<env>.yaml` (git-crypt). The file must exist. |
| `--encryption` | `sops` | Encryption backend: `sops` or `git-crypt` |
| `--dry-run` | `false` | Print manifests without applying |
| `--dry-run-output` | — | Write dry-run manifests to a file (JSON output) |
| `--skip-argocd-install` | `false` | Skip the Helm ArgoCD installation |
| `--enable-cilium` | `false` | Install Cilium in `kube-system`, wait until healthy, then configure it for ArgoCD management |
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig file |
| `--context` | current context | Kubeconfig context to use |
| `--age-key-file` | `SOPS_AGE_KEY_FILE` env | Path to age private key for SOPS decryption or for `--store-sops-age-key`. |
| `--store-sops-age-key` | `false` | Store the age private key from `--age-key-file` (or `SOPS_AGE_KEY_FILE`) as `sops-age-key` in `argocd`. Use when ArgoCD decrypts SOPS values in-cluster. Requires `--encryption sops`, and the key path is validated before any cluster change. |
| `--gitcrypt-key-file` | — | Path to git-crypt symmetric key file. When provided, stores the key as a `git-crypt-key` K8s Secret in the `argocd` namespace |
| `--app-path` | `apps` | Path inside the Git repo for the App of Apps source (used in the ArgoCD Application CR `spec.source.path`). If `apps` does not exist and no value is provided, the CLI auto-detects a matching chart (Chart.yaml + templates/application.yaml). |
| `--wait-for-health` | `false` | Wait for cluster components (ArgoCD, Vault, External Secrets) to be ready after bootstrap |
| `--health-timeout` | `180` | Timeout in seconds for health checks (default 180 = 3 minutes) |
| `--report-format` | `summary` | Report format: `summary`, `json`, or `none` |
| `--report-output` | — | Write JSON report to file |
| `--force` | `false` | Bootstrap even if the cluster already has an App of Apps, overwriting it. Without this flag, bootstrap stops on an already bootstrapped cluster |
| `--yes`, `-y` | `false` | Skip the 10 second countdown before the cluster is modified. Already skipped when stdout is not a terminal |

When `--enable-cilium` and `--skip-argocd-install` are combined, the CLI waits for the existing `argocd-server` deployment before applying the App of Apps with Cilium enabled.

## Examples

```bash
# SOPS (default)
cluster-bootstrap-cli bootstrap dev

# Bootstrap Cilium before ArgoCD
cluster-bootstrap-cli bootstrap dev --enable-cilium

# Let ArgoCD decrypt SOPS-encrypted Helm values in-cluster
cluster-bootstrap-cli bootstrap prod \
  --age-key-file ./age-key.txt \
  --store-sops-age-key

# git-crypt
cluster-bootstrap-cli bootstrap dev --encryption git-crypt

# git-crypt with key stored in cluster + custom app path
cluster-bootstrap-cli bootstrap dev \
  --encryption git-crypt \
  --gitcrypt-key-file ./git-crypt-key \
  --app-path k8s/apps

# Dry run to a file
cluster-bootstrap-cli bootstrap dev --dry-run --dry-run-output /tmp/bootstrap.json

# Re-run against an already bootstrapped cluster, without the countdown
cluster-bootstrap-cli bootstrap dev --force --yes

# Repo content in a subdirectory
# First, update apps/values.yaml to set repo.basePath: "k8s"

# Option 1: From repository root with --base-dir
./k8s/cli/cluster-bootstrap --base-dir ./k8s bootstrap dev \
  --app-path k8s/apps \
  --wait-for-health -v

# Option 2: From inside subdirectory (auto-detected)
cd k8s
cluster-bootstrap-cli bootstrap dev \
  --app-path apps \
  --wait-for-health -v

# Wait for components to be ready (with 5-minute timeout)
cluster-bootstrap-cli bootstrap dev --wait-for-health --health-timeout 300

# Wait for health with verbose output
cluster-bootstrap-cli bootstrap dev --wait-for-health -v
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
cluster-bootstrap-cli bootstrap dev

# JSON report to stdout (for piping to jq, logging systems, etc.)
cluster-bootstrap-cli bootstrap dev --report-format json

# Save JSON report to file for later analysis
cluster-bootstrap-cli bootstrap dev --report-output bootstrap-report.json

# JSON to both stdout and file
cluster-bootstrap-cli bootstrap dev --report-format json --report-output bootstrap-report.json

# Suppress report output (show only progress messages)
cluster-bootstrap-cli bootstrap dev --report-format none

# Full bootstrap with health checks and report
cluster-bootstrap-cli bootstrap dev --wait-for-health --report-output bootstrap-$(date +%Y%m%d-%H%M%S).json
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
