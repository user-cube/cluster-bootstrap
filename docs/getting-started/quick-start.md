# Quick Start

This guide walks through bootstrapping a cluster from scratch.

## 1. Install the CLI

**Option A: Homebrew (recommended)**

```bash
brew install user-cube/tap/cluster-bootstrap-cli
```

**Option B: Install globally with `go install`**

```bash
# From GitHub
go install github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli@latest

# Verify installation
cluster-bootstrap-cli --help
```

**Option C: Install from local source**

```bash
# Clone the repository
git clone git@github.com:user-cube/cluster-bootstrap.git
cd cluster-bootstrap

# Install globally
go install ./cluster-bootstrap-cli
```

**Option D: Build locally**

```bash
task build
# Binary will be at: cluster-bootstrap-cli/cluster-bootstrap-cli
./cluster-bootstrap-cli/cluster-bootstrap-cli --help
```

## 2. Initialize secrets (first time only)

Run the interactive init command to configure encryption and create per-environment secrets files:

```bash
./cluster-bootstrap-cli/cluster-bootstrap-cli init
```

This will:

1. Prompt you to choose an encryption provider (age, AWS KMS, GCP KMS, or git-crypt)
2. For SOPS providers: collect the encryption key, generate `.sops.yaml`, create encrypted `secrets.<env>.enc.yaml` files
3. For git-crypt: verify `git-crypt init` has been run, update `.gitattributes`, create plaintext `secrets.<env>.yaml` files

### Using git-crypt instead of SOPS

```bash
git-crypt init
./cluster-bootstrap-cli/cluster-bootstrap-cli init --provider git-crypt
```

## 3. Bootstrap the cluster

Run the bootstrap command with your target environment:

```bash
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev
```

This performs the following steps:

1. Loads secrets — decrypts via SOPS (default) or reads plaintext git-crypt files
2. Creates the `argocd` namespace
3. Creates the `repo-ssh-key` Secret with your Git SSH credentials
4. Installs ArgoCD via Helm (using `components/argocd/` chart and values)
5. Deploys the App of Apps root Application
6. Prints ArgoCD access instructions

### Common flags

```bash
# Use a specific secrets file
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev --secrets-file ./my-secrets.enc.yaml

# Use a specific kubeconfig or context
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev --kubeconfig ~/.kube/my-config --context my-cluster

# Specify age key location (SOPS)
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev --age-key-file ./age-key.txt

# Use git-crypt encryption
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev --encryption git-crypt

# git-crypt with key stored in cluster
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev --encryption git-crypt --gitcrypt-key-file ./git-crypt-key

# Dry run — print manifests without applying
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev --dry-run

# Dry run — write manifests to a file
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev --dry-run --dry-run-output /tmp/bootstrap.json

# Skip ArgoCD Helm install (if already installed)
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev --skip-argocd-install

# Repo content in a subdirectory with custom app path
# First, update apps/values.yaml to set repo.basePath: "k8s"
./cluster-bootstrap-cli/cluster-bootstrap-cli --base-dir ./k8s bootstrap dev --app-path k8s/apps

# Wait for components to be ready after bootstrap
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev --wait-for-health

# Wait for health with longer timeout (5 minutes)
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev --wait-for-health --health-timeout 300
```

Note: when using `--secrets-file` or the auto-detected secrets path, the file must already exist.

### Waiting for components to be ready

Use `--wait-for-health` to verify that critical components (ArgoCD, Vault, External Secrets) are ready after bootstrap completes:

```bash
./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap dev --wait-for-health
```

This will poll the cluster every 2 seconds for up to 180 seconds (3 minutes) and display a health status report showing which components are ready.

## 4. Access ArgoCD

After bootstrap completes, access the ArgoCD UI:

```bash
kubectl port-forward svc/argocd-server -n argocd 8080:443
```

Get the initial admin password:

```bash
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d
```

Open [https://localhost:8080](https://localhost:8080) and log in with `admin` and the password above.

## 5. Store Vault token (non-dev environments)

For staging and production, after Vault initializes you need to store the root token:

```bash
./cluster-bootstrap-cli/cluster-bootstrap-cli vault-token --token <vault-root-token>
echo "<vault-root-token>" | ./cluster-bootstrap-cli/cluster-bootstrap-cli vault-token
./cluster-bootstrap-cli/cluster-bootstrap-cli vault-token
```

This creates a `vault-root-token` Secret in the `vault` namespace, which the Vault configuration and seed jobs use.

## What happens next?

Once ArgoCD is running with the App of Apps deployed, it will automatically sync all enabled components in sync wave order:

1. **Wave 0**: ArgoCD (self-manages)
2. **Wave 1**: Vault, External Secrets
3. **Wave 2**: Prometheus Operator CRDs, ArgoCD Repo Secret, Reloader
4. **Wave 3**: Kube Prometheus Stack, Trivy Operator

All components use automated sync with pruning and self-healing enabled.
