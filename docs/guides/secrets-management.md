# Secrets Management

This repo uses a multi-layer secrets architecture with two supported encryption backends for secrets at rest: **SOPS** (default) and **git-crypt**. At runtime, Vault provides secrets storage and External Secrets Operator syncs secrets into Kubernetes.

## Overview

```
┌─────────────────────────────────────────────────────────┐
│                    Developer Machine                      │
│                                                           │
│  Option A: SOPS                                           │
│  secrets.dev.enc.yaml ──(SOPS + age)──> plaintext YAML   │
│                                                           │
│  Option B: git-crypt                                      │
│  secrets.dev.yaml ──(git-crypt unlock)──> plaintext YAML  │
│                                                           │
│  CLI bootstrap reads decrypted secrets and:               │
│    1. Creates repo-ssh-key Secret in argocd namespace     │
│    2. Vault seed job copies SSH key into Vault KV         │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                      │
│                                                           │
│  Vault (KV Store)                                         │
│    └── SSH private key                                    │
│           │                                               │
│           ▼                                               │
│  External Secrets Operator                                │
│    └── ExternalSecret (watches Vault)                     │
│           │                                               │
│           ▼                                               │
│  Kubernetes Secret (argocd namespace)                     │
│    └── ArgoCD uses for Git repo access                    │
└─────────────────────────────────────────────────────────┘
```

## SOPS + Age Encryption

[SOPS](https://github.com/getsops/sops) encrypts YAML files so secrets can be stored in Git safely. This repo uses [age](https://github.com/FiloSottile/age) as the encryption backend.

### Configuration

`.sops.yaml` defines encryption rules:

```yaml
creation_rules:
  - path_regex: \.enc\.yaml$
    age: age1wj3m2ayk4a8nwxc8r678l06q4h4xxa0gqa2l6eyqf037wcdgxaqqla9fr8
```

Any file matching `*.enc.yaml` will be encrypted with the specified age public key.

### Encrypted secrets structure

Each environment has a `secrets.<env>.enc.yaml` file containing:

```yaml
repo:
  url: git@github.com:user-cube/cluster-bootstrap.git
  targetRevision: main
  sshPrivateKey: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    ...
    -----END OPENSSH PRIVATE KEY-----
```

### Working with encrypted files

```bash
# Decrypt a file (requires age-key.txt)
SOPS_AGE_KEY_FILE=./age-key.txt sops -d secrets.dev.enc.yaml

# Encrypt a plaintext file
sops -e secrets.dev.yaml > secrets.dev.enc.yaml

# Edit in-place
SOPS_AGE_KEY_FILE=./age-key.txt sops secrets.dev.enc.yaml
```

### Initialize with the CLI

The `init` command sets up SOPS and creates encrypted secrets interactively:

```bash
./cli/cluster-bootstrap init --provider age --age-key-file ./age-key.txt
```

This supports age, AWS KMS, and GCP KMS as encryption providers.

## git-crypt Encryption

[git-crypt](https://github.com/AGWA/git-crypt) provides transparent file encryption in Git repositories. Files are encrypted on commit and decrypted on checkout — no separate decrypt step is needed during development.

### Setup

```bash
# Initialize git-crypt in the repo (one-time)
git-crypt init

# Run the CLI init with git-crypt provider
./cli/cluster-bootstrap init --provider git-crypt
```

This will:

1. Verify that `git-crypt init` has been run
2. Add the encryption pattern to `.gitattributes`:
   ```
   secrets.*.yaml filter=git-crypt diff=git-crypt
   ```
3. Create plaintext `secrets.<env>.yaml` files (encrypted automatically on commit)

### Secrets file structure

git-crypt secrets files use the same structure as SOPS but without the `.enc` suffix:

```yaml
# secrets.dev.yaml (plaintext locally, encrypted in Git)
repo:
  url: git@github.com:user-cube/cluster-bootstrap.git
  targetRevision: main
  sshPrivateKey: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    ...
    -----END OPENSSH PRIVATE KEY-----
```

### Bootstrap with git-crypt

```bash
# Ensure the repo is unlocked
git-crypt unlock

# Bootstrap using git-crypt backend
./cli/cluster-bootstrap bootstrap dev --encryption git-crypt
```

If ArgoCD needs to decrypt the repo, store the symmetric key as a K8s secret:

```bash
# Export the git-crypt key
git-crypt export-key /tmp/git-crypt-key

# Store it in the cluster
./cli/cluster-bootstrap gitcrypt-key --key-file /tmp/git-crypt-key
```

### SOPS vs git-crypt

| | SOPS | git-crypt |
|---|------|-----------|
| Encryption granularity | Per-value (only values are encrypted) | Per-file (entire file is encrypted) |
| Key management | age, AWS KMS, GCP KMS | Symmetric key + GPG |
| Git diff | Partial (metadata visible) | Binary diff when locked |
| Setup complexity | Requires `.sops.yaml` config | Requires `git-crypt init` + GPG/key sharing |
| Best for | CI/CD pipelines, multi-cloud | Simple setups, small teams |

## Vault Integration

[Vault](https://www.hashicorp.com/products/vault) provides runtime secrets storage in the cluster.

### Bootstrap flow

1. The CLI creates an initial `repo-ssh-key` Kubernetes Secret during bootstrap
2. Vault starts and the seed job copies the SSH key from the Kubernetes Secret into Vault's KV store
3. The config job sets up Kubernetes authentication in Vault

### Non-dev environments

For staging and production, Vault requires initialization:

```bash
# After Vault pods are running
kubectl exec -n vault vault-0 -- vault operator init

# Store the root token
./cli/cluster-bootstrap vault-token --token <root-token>
```

## External Secrets Operator

The [External Secrets Operator](https://external-secrets.io/) bridges Vault and Kubernetes Secrets.

### Components

- **SecretStore** — configures the connection to Vault (address, auth method)
- **ExternalSecret** — defines what to fetch from Vault and where to store it in Kubernetes

### ArgoCD Repo Secret flow

The `argocd-repo-secret` component creates:

1. A `SecretStore` pointing to Vault with Kubernetes auth
2. An `ExternalSecret` that fetches the SSH key from Vault KV
3. The operator creates a Kubernetes Secret in the `argocd` namespace
4. ArgoCD uses this Secret for Git repository access

This closes the loop — after bootstrap, credential rotation flows through Vault and External Secrets without manual intervention.
