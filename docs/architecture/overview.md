# Architecture Overview

## App of Apps Pattern

This repository uses ArgoCD's **App of Apps** pattern. A single root Application (`app-of-apps`) generates child Application resources for every platform component.

```
┌──────────────────────────────────────────────────────┐
│                   Bootstrap CLI                       │
│  1. Install ArgoCD (Helm)                            │
│  2. Create repo SSH secret                           │
│  3. Apply app-of-apps Application CR                 │
└──────────────────┬───────────────────────────────────┘
                   │
                   ▼
┌──────────────────────────────────────────────────────┐
│              App of Apps (apps/)                       │
│  Helm chart that templates child Applications         │
│  Values per environment: dev / staging / prod         │
└──────────────────┬───────────────────────────────────┘
                   │
        ┌──────────┼──────────┬──────────┐
        ▼          ▼          ▼          ▼
   ┌─────────┐ ┌───────┐ ┌────────┐ ┌────────┐
   │ ArgoCD  │ │ Vault │ │Ext Sec │ │  ...   │
   │ Wave 0  │ │Wave 1 │ │Wave 1  │ │        │
   └─────────┘ └───────┘ └────────┘ └────────┘
```

## Sync Wave Ordering

ArgoCD sync waves ensure components deploy in dependency order:

| Wave | Components | Rationale |
|------|-----------|-----------|
| 0 | ArgoCD | Must be running first (self-managed) |
| 1 | Vault, External Secrets | Secrets infrastructure needed before consumers |
| 2 | Prometheus Operator CRDs, ArgoCD Repo Secret, Reloader | CRDs before stack; repo credentials for ArgoCD |
| 3 | Kube Prometheus Stack, Trivy Operator | Depend on CRDs and secrets being available |

Sync waves are configured via the `syncWave` property of each component in `apps/values.yaml`. The dynamic template applies them as `argocd.argoproj.io/sync-wave` annotations.

## Self-Managed ArgoCD

ArgoCD manages its own deployment through the App of Apps:

1. The CLI performs the **initial Helm install** of ArgoCD
2. The App of Apps includes an ArgoCD Application (wave 0) pointing to `components/argocd/`
3. ArgoCD detects itself as a managed resource and keeps it in sync
4. Future ArgoCD upgrades happen by updating `components/argocd/Chart.yaml` and pushing to Git

This means after the initial bootstrap, **all changes flow through Git** — including ArgoCD configuration changes.

## Automated Sync Policy

All child Applications are configured with:

- **Automated sync** — changes in Git are applied automatically
- **Prune** — resources removed from Git are deleted from the cluster
- **Self-heal** — manual changes in the cluster are reverted to match Git
- **Create namespace** — target namespaces are created if they don't exist

## Values Cascade

Each component uses a two-layer values strategy:

1. `values/base.yaml` — shared defaults across all environments
2. `values/<env>.yaml` — environment-specific overrides (replicas, resources, feature flags)

ArgoCD merges these in order, with environment values taking precedence. See [Environments](../guides/environments.md) for details.
