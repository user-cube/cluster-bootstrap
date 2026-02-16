# CLI Tool

The `cluster-bootstrap` CLI automates cluster bootstrapping. Built in Go with [Cobra](https://github.com/spf13/cobra), it handles secret decryption, Helm installation, and Kubernetes resource creation.

## Building

```bash
cd cli
task build
```

This runs `go mod tidy` and builds the `cluster-bootstrap` binary in the `cli/` directory.

### Other Taskfile commands

| Command | Description |
|---------|-------------|
| `task build` | Build the binary |
| `task clean` | Remove the binary |
| `task tidy` | Run `go mod tidy` |
| `task fmt` | Format Go source files |
| `task vet` | Run `go vet` |

## Global Flags

These flags are available on all commands:

| Flag | Default | Description |
|------|---------|-------------|
| `--base-dir` | `.` | Base directory for repo content. Use when K8s manifests live in a subdirectory (e.g. `k8s/`). Affects local file resolution only (Chart.yaml, values, secrets files). |
| `-v, --verbose` | `false` | Enable verbose output |

## Commands

### `bootstrap <environment>`

Performs the full cluster bootstrap sequence.

```bash
./cli/cluster-bootstrap bootstrap dev
```

**What it does:**

1. Loads secrets — decrypts via SOPS (default) or reads plaintext git-crypt files
2. Creates the `argocd` namespace
3. Creates the `repo-ssh-key` Secret with Git SSH credentials
4. Optionally creates `git-crypt-key` Secret (if `--gitcrypt-key-file` provided)
5. Installs ArgoCD via Helm (from `components/argocd/`)
6. Deploys the App of Apps root Application
7. Prints ArgoCD access instructions

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--secrets-file` | auto | Path to secrets file. Auto-detected based on `--encryption`: `secrets.<env>.enc.yaml` (sops) or `secrets.<env>.yaml` (git-crypt) |
| `--encryption` | `sops` | Encryption backend: `sops` or `git-crypt` |
| `--dry-run` | `false` | Print manifests without applying |
| `--skip-argocd-install` | `false` | Skip the Helm ArgoCD installation |
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig file |
| `--context` | current context | Kubeconfig context to use |
| `--age-key-file` | `SOPS_AGE_KEY_FILE` env | Path to age private key (SOPS only) |
| `--gitcrypt-key-file` | — | Path to git-crypt symmetric key file. When provided, stores the key as a `git-crypt-key` K8s Secret in the `argocd` namespace |
| `--app-path` | `apps` | Path inside the Git repo for the App of Apps source (used in the ArgoCD Application CR `spec.source.path`) |

**Examples:**

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

# Repo content in a subdirectory
./cli/cluster-bootstrap --base-dir ./k8s bootstrap dev --app-path k8s/apps
```

### `init`

Interactive setup to configure encryption and create per-environment secrets files.

```bash
./cli/cluster-bootstrap init
```

**What it does:**

1. Prompts for encryption provider (age, AWS KMS, GCP KMS, or git-crypt)
2. For SOPS providers: collects the encryption key, generates `.sops.yaml`, creates encrypted `secrets.<env>.enc.yaml` files
3. For git-crypt: verifies `git-crypt init` has been run, ensures `.gitattributes` has the git-crypt pattern, creates plaintext `secrets.<env>.yaml` files (encrypted transparently on commit)

**Flags:**

| Flag | Description |
|------|-------------|
| `--provider` | Encryption provider: `age`, `aws-kms`, `gcp-kms`, or `git-crypt` |
| `--age-key-file` | Path to age public key file |
| `--kms-arn` | AWS KMS key ARN |
| `--gcp-kms-key` | GCP KMS key resource ID |
| `--output-dir` | Output directory (default: current directory, or `--base-dir` if set) |

### `vault-token`

Stores the Vault root token as a Kubernetes Secret.

```bash
./cli/cluster-bootstrap vault-token --token <root-token>
```

**What it does:**

Creates or updates a `vault-root-token` Secret in the `vault` namespace. This is required for non-dev Vault instances after running `vault operator init`.

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `--token` | Yes | Vault root token |
| `--kubeconfig` | No | Path to kubeconfig file |
| `--context` | No | Kubeconfig context to use |

### `gitcrypt-key`

Stores a git-crypt symmetric key as a Kubernetes Secret.

```bash
./cli/cluster-bootstrap gitcrypt-key --key-file ./git-crypt-key
```

**What it does:**

Reads a git-crypt symmetric key file and creates or updates a `git-crypt-key` Secret in the `argocd` namespace. This allows ArgoCD (with appropriate plugins) to decrypt git-crypt encrypted repositories.

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `--key-file` | Yes | Path to git-crypt symmetric key file |
| `--kubeconfig` | No | Path to kubeconfig file |
| `--context` | No | Kubeconfig context to use |

## Dependencies

The CLI uses these key libraries:

| Library | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/charmbracelet/huh` | Interactive terminal UI |
| `github.com/getsops/sops/v3` | SOPS encryption/decryption |
| `helm.sh/helm/v3` | Helm SDK for chart installation |
| `k8s.io/client-go` | Kubernetes API client |
