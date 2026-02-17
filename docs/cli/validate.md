# validate

```bash
./cli/cluster-bootstrap validate <environment>
```

Validates local configuration, secrets, and optional cluster access. This is a deeper check than `doctor`.

## What it does

1. Validates base directory and app path
2. Verifies `kubectl` and `helm`
3. Checks current context and optional cluster access
4. Validates encryption tooling
5. Reads and validates secrets files
6. Checks `.sops.yaml` rules or `.gitattributes` patterns

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--encryption` | `sops` | Encryption backend: `sops` or `git-crypt` |
| `--secrets-file` | auto | Path to secrets file (defaults to `secrets.<env>.enc.yaml` or `secrets.<env>.yaml`) |
| `--age-key-file` | — | Path to age private key (SOPS only) |
| `--app-path` | `apps` | Path inside the Git repo for the App of Apps source |
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig file |
| `--context` | current context | Kubeconfig context to use |
| `--skip-cluster-check` | `false` | Skip cluster access checks |

## Examples

```bash
# Default checks (SOPS)
./cli/cluster-bootstrap validate dev

# git-crypt checks
./cli/cluster-bootstrap validate dev --encryption git-crypt

# Skip cluster checks
./cli/cluster-bootstrap validate dev --skip-cluster-check

# Use a specific kubeconfig and context
./cli/cluster-bootstrap validate dev --kubeconfig ~/.kube/my-config --context my-cluster
```
