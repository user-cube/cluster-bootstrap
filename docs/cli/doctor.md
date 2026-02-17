# doctor

```bash
./cli/cluster-bootstrap-cli doctor
```

Checks local tooling and optionally validates cluster access.

## What it does

1. Verifies `kubectl` is installed
2. Prints the current kubectl context
3. Optionally checks cluster access (`kubectl cluster-info`)
4. Verifies `helm` is installed
5. Verifies encryption tooling (`sops` and `age`, or `git-crypt`)

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--encryption` | `sops` | Encryption backend: `sops` or `git-crypt` |
| `--age-key-file` | — | Path to age private key (SOPS only) |
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig file |
| `--context` | current context | Kubeconfig context to use |
| `--skip-cluster-check` | `false` | Skip cluster access checks |

## Examples

```bash
# Default checks (SOPS)
./cli/cluster-bootstrap-cli doctor

# git-crypt checks
./cli/cluster-bootstrap-cli doctor --encryption git-crypt

# Skip cluster checks (tooling only)
./cli/cluster-bootstrap-cli doctor --skip-cluster-check

# Use a specific kubeconfig and context
./cli/cluster-bootstrap-cli doctor --kubeconfig ~/.kube/my-config --context my-cluster
```
