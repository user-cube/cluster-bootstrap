# Repository Structure

```
cluster-bootstrap/
├── apps/                          # App of Apps Helm chart
│   ├── Chart.yaml                 # Chart metadata (no dependencies)
│   ├── values.yaml                # Default component definitions
│   ├── templates/
│   │   └── application.yaml       # Dynamic template (iterates over components)
│   └── values/                    # Per-environment overrides
│       ├── dev.yaml
│       ├── staging.yaml
│       └── prod.yaml
├── components/                    # Individual component Helm charts
│   ├── argocd/
│   ├── vault/
│   ├── external-secrets/
│   ├── argocd-repo-secret/
│   ├── prometheus-operator-crds/
│   ├── kube-prometheus-stack/
│   ├── reloader/
│   └── trivy-operator/
├── cli/                           # Go CLI tool
│   ├── main.go
│   ├── cmd/                       # Cobra commands
│   ├── internal/                  # Internal packages
│   ├── Taskfile.yml
│   ├── go.mod
│   └── go.sum
├── docs/                          # Documentation (this site)
├── mkdocs.yml                     # MkDocs configuration
├── .gitignore
├── .sops.yaml                     # SOPS encryption rules
└── README.md
```

## `apps/` — App of Apps

The root Helm chart that ArgoCD deploys. A single dynamic template (`apps/templates/application.yaml`) iterates over the `components` map defined in `apps/values.yaml` and generates one ArgoCD `Application` resource per enabled component.

`apps/values.yaml` defines all components with their properties:

- `enabled` — whether to deploy this component (default: `true`)
- `namespace` — target Kubernetes namespace
- `syncWave` — ArgoCD sync wave for deployment ordering
- `hasValues` — whether the component uses Helm value files (default: `true`)
- `createNamespace` — whether to add `CreateNamespace=true` syncOption (default: `true`)
- `syncOptions` — additional syncOptions (e.g., `ServerSideApply=true`)
- `ignoreDifferences` — ArgoCD ignoreDifferences configuration

The `apps/values/` environment files only need to set the `environment` key. To disable a component for a specific environment, override its `enabled` flag:

```yaml
environment: dev
components:
  trivy-operator:
    enabled: false
```

Helm performs a deep merge, so all other properties inherit from `apps/values.yaml`.

## `components/` — Platform Components

Each subdirectory is a standalone Helm chart (or chart wrapper) for one platform component. The common structure is:

```
components/<name>/
├── Chart.yaml          # Declares upstream chart dependency
├── templates/          # Optional custom templates
└── values/
    ├── base.yaml       # Shared defaults
    ├── dev.yaml        # Dev overrides
    ├── staging.yaml    # Staging overrides
    └── prod.yaml       # Prod overrides
```

Most components are thin wrappers around upstream Helm charts — `Chart.yaml` declares the dependency and `values/` files configure it. Some components (like Vault and ArgoCD Repo Secret) include custom templates for additional resources.

## `cli/` — Bootstrap CLI

A Go application that automates cluster bootstrapping. Built with Cobra (CLI framework), it handles SOPS decryption, Helm installation, and Kubernetes resource creation. See the [CLI documentation](../cli/index.md) for details.

## Config Files

| File | Purpose |
|------|---------|
| `.sops.yaml` | SOPS encryption rules — defines which files to encrypt and with which key |
| `.gitignore` | Ignores charts/, secrets, binaries, IDE files, and MkDocs build output |
| `age-key.txt` | Age private key for SOPS decryption (gitignored) |
