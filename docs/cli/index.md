# CLI Tool

The `cluster-bootstrap-cli` automates cluster bootstrapping. Built in Go with [Cobra](https://github.com/spf13/cobra), it handles secret decryption, Helm installation, and Kubernetes resource creation.

## Installation

### Homebrew (recommended)

```bash
brew install user-cube/tap/cluster-bootstrap-cli
```

### Using `go install`

**From local source:**

```bash
# Clone the repository
git clone git@github.com:user-cube/cluster-bootstrap.git
cd cluster-bootstrap

# Install globally
go install ./cluster-bootstrap-cli
```

**From GitHub:**

```bash
go install github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli@latest
```

This installs `cluster-bootstrap-cli` in your `$GOPATH/bin` (typically `~/go/bin`). Make sure this directory is in your `$PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Verify installation:

```bash
cluster-bootstrap-cli --help
```

### Building from source

For development or customization:

```bash
# Option 1: Build locally
task build
# Creates: cluster-bootstrap-cli/cluster-bootstrap-cli

# Option 2: Install from local source
task install
# Installs to: $(go env GOPATH)/bin/cluster-bootstrap-cli
```

### Other Taskfile commands

| Command | Description |
|---------|-------------|
| `task build` | Build the binary locally |
| `task install` | Build and install to GOPATH/bin |
| `task clean` | Remove the binary |
| `task tidy` | Run `go mod tidy` |
| `task fmt` | Format Go source files |
| `task vet` | Run `go vet` |
| `task test` | Run tests |

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
| [`template`](template.md) | Customize the template with your organization and repository |
| [`doctor`](doctor.md) | Check local tools and cluster access |
| [`status`](status.md) | Show cluster status and component information |
| [`validate`](validate.md) | Validate local config and secrets |
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
