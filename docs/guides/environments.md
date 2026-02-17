# Environments

The repo supports three environments: **dev**, **staging**, and **prod**. Each environment uses the same components but with different resource allocations and configuration.

## Environment Comparison

| Aspect | Dev | Staging | Prod |
|--------|-----|---------|------|
| Purpose | Local development | Pre-production testing | Production workloads |
| Replicas | Minimal (1) | Moderate (1-2) | High availability (2-3) |
| Resources | Low requests/limits | Moderate | Substantial |
| Vault mode | Dev (in-memory) | Standalone (persistent) | HA with Raft |
| Prometheus retention | 2h | 4h | 6h |
| Trivy concurrent scans | 2 | 3 | 5 |

## Values Cascade

Each component uses two values files applied in order:

1. **`values/base.yaml`** — shared defaults (features, integrations, non-resource settings)
2. **`values/<env>.yaml`** — environment-specific overrides (replicas, resources, feature flags)

Environment values take precedence. Helm performs a deep merge, so environment files only need to specify the keys they override.

### Example

`values/base.yaml`:
```yaml
external-secrets:
  installCRDs: true
  webhook:
    replicaCount: 1
  certController:
    replicaCount: 1
```

`values/prod.yaml`:
```yaml
external-secrets:
  replicaCount: 3
  webhook:
    replicaCount: 3
  certController:
    replicaCount: 3
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
```

Result for prod: CRDs are installed (from base), 3 replicas with explicit resources (from prod).

## Resource Scaling Strategy

Resources scale across environments following a consistent pattern:

| | Dev | Staging | Prod |
|---|-----|---------|------|
| CPU requests | 10-50m | 50-200m | 100-500m |
| Memory requests | 32-128Mi | 64-512Mi | 128Mi-1Gi |
| CPU limits | 100-200m | 100-500m | 250m-1000m |
| Memory limits | 128-256Mi | 256Mi-1Gi | 512Mi-2Gi |

Dev environments use minimal resources suitable for local clusters (kind, minikube). Production allocates enough for reliability under load.

## App of Apps Values

Component defaults (namespace, sync wave, syncOptions, etc.) are defined in `apps/values.yaml`. The environment files in `apps/values/` only need to set the environment name:

```yaml
# apps/values/dev.yaml
environment: dev
```

All components are enabled by default. To disable a component for a specific environment, override its `enabled` flag:

```yaml
# apps/values/dev.yaml
environment: dev
components:
  trivy-operator:
    enabled: false
```

Helm performs a deep merge, so only the overridden properties change — the rest inherits from `apps/values.yaml`.

## Adding a New Environment

1. Create `apps/values/<env>.yaml` with `environment: <env>`
2. Create `values/<env>.yaml` in each component under `components/`
3. Create `secrets.<env>.enc.yaml` with encrypted credentials
4. Bootstrap with `./cluster-bootstrap-cli/cluster-bootstrap-cli bootstrap <env>`
