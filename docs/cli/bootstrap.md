# bootstrap

```bash
./cli/cluster-bootstrap bootstrap <environment>
```

Performs the full cluster bootstrap sequence.

```bash
./cli/cluster-bootstrap bootstrap dev
```

## What it does

1. Loads secrets — decrypts via SOPS (default) or reads plaintext git-crypt files
2. Creates the `argocd` namespace
3. Creates the `repo-ssh-key` Secret with Git SSH credentials
4. Optionally creates `git-crypt-key` Secret (if `--gitcrypt-key-file` provided)
5. Installs ArgoCD via Helm (from `components/argocd/`)
6. Deploys the App of Apps root Application
7. Prints ArgoCD access instructions

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

## Examples

```bash
# SOPS (default)
./cli/cluster-bootstrap bootstrap dev

# git-crypt
./cli/cluster-bootstrap bootstrap dev --encryption git-crypt

# git-crypt with key stored in cluster + custom app path
./cli/cluster-bootstrap bootstrap dev \
  --encryption git-crypt \
  --gitcrypt-key-file ./git-crypt-key \
  --app-path k8s/apps

# Dry run to a file
./cli/cluster-bootstrap bootstrap dev --dry-run --dry-run-output /tmp/bootstrap.json

# Repo content in a subdirectory
./cli/cluster-bootstrap --base-dir ./k8s bootstrap dev --app-path k8s/apps
```
