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

| Command | Description |
|---------|-------------|
| [`bootstrap`](bootstrap.md) | Full cluster bootstrap sequence |
| [`init`](init.md) | Interactive encryption setup |
| [`vault-token`](vault-token.md) | Store Vault root token as K8s Secret |
| [`gitcrypt-key`](gitcrypt-key.md) | Store git-crypt key as K8s Secret |

## Dependencies

The CLI uses these key libraries:

| Library | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/charmbracelet/huh` | Interactive terminal UI |
| `github.com/getsops/sops/v3` | SOPS encryption/decryption |
| `helm.sh/helm/v3` | Helm SDK for chart installation |
| `k8s.io/client-go` | Kubernetes API client |
