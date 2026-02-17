# vault-token

```bash
cluster-bootstrap-cli vault-token --token <root-token>
echo "<root-token>" | cluster-bootstrap-cli vault-token
cluster-bootstrap-cli vault-token
```

Stores the Vault root token as a Kubernetes Secret.

## What it does

Creates or updates a `vault-root-token` Secret in the `vault` namespace. This is required for non-dev Vault instances after running `vault operator init`.

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| `--token` | No | Vault root token (can be read from stdin or prompt) |
| `--kubeconfig` | No | Path to kubeconfig file |
| `--context` | No | Kubeconfig context to use |
