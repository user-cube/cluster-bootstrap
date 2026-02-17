# gitcrypt-key

```bash
cluster-bootstrap-cli gitcrypt-key --key-file ./git-crypt-key
```

Stores a git-crypt symmetric key as a Kubernetes Secret.

## What it does

Reads a git-crypt symmetric key file and creates or updates a `git-crypt-key` Secret in the `argocd` namespace. This allows ArgoCD (with appropriate plugins) to decrypt git-crypt encrypted repositories.

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| `--key-file` | Yes | Path to git-crypt symmetric key file |
| `--kubeconfig` | No | Path to kubeconfig file |
| `--context` | No | Kubeconfig context to use |
