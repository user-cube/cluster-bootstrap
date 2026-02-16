# vault-token

```bash
./cli/cluster-bootstrap vault-token --token <root-token>
```

Stores the Vault root token as a Kubernetes Secret.

## What it does

Creates or updates a `vault-root-token` Secret in the `vault` namespace. This is required for non-dev Vault instances after running `vault operator init`.

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| `--token` | Yes | Vault root token |
| `--kubeconfig` | No | Path to kubeconfig file |
| `--context` | No | Kubeconfig context to use |
