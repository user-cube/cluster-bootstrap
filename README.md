# Cluster Bootstrap

GitOps repo for bootstrapping Kubernetes clusters with ArgoCD using the **App of Apps** pattern.

## Prerequisites

- `kubectl` configured with access to the target cluster
- SSH private key with read access to this repo
- `helm` (for local template testing)

## Quick Start

```bash
export REPO_URL="git@github.com:org/cluster-bootstrap.git"
export SSH_KEY_PATH="$HOME/.ssh/id_ed25519"

./bootstrap/install.sh dev
```

This will:

1. Install ArgoCD in the `argocd` namespace
2. Create the SSH credentials secret for repo access
3. Deploy the root **App of Apps** Application for the specified environment

## Access ArgoCD UI

```bash
kubectl port-forward svc/argocd-server -n argocd 8080:443
```

Get the initial admin password:

```bash
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d
```

## Architecture

```
bootstrap/install.sh  →  ArgoCD + App of Apps (root Application)
                              ↓
                         apps/ (Helm chart generating Application CRs)
                              ↓
                    components/argocd/  (self-managed ArgoCD)
                    components/xxx/    (other components)
```

ArgoCD manages itself — changes pushed to this repo are automatically synced.

## Adding a New Component

1. Create the component chart:

```
components/my-component/
├── Chart.yaml          # Helm dependency on the upstream chart
└── values/
    ├── base.yaml       # Shared config
    ├── dev.yaml
    ├── staging.yaml
    └── prod.yaml
```

2. Create the Application template in `apps/templates/my-component.yaml`:

```yaml
{{- if .Values.myComponent.enabled }}
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-component
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: default
  source:
    repoURL: {{ .Values.repo.url }}
    targetRevision: {{ .Values.repo.targetRevision }}
    path: components/my-component
    helm:
      valueFiles:
        - values/base.yaml
        - values/{{ .Values.environment }}.yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: my-component
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
{{- end }}
```

3. Enable in each environment's values file (`apps/values/{env}.yaml`):

```yaml
myComponent:
  enabled: true
```

## Environments

| Environment | Values File | Description |
|-------------|-------------|-------------|
| dev | `apps/values/dev.yaml` | Local/development clusters, minimal resources |
| staging | `apps/values/staging.yaml` | Pre-production, moderate resources |
| prod | `apps/values/prod.yaml` | Production, HA configuration |
