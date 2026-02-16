# Adding a Component

This guide walks through adding a new platform component to the stack.

## 1. Create the component chart

Create a new directory under `components/`:

```
components/my-component/
├── Chart.yaml
└── values/
    ├── base.yaml
    ├── dev.yaml
    ├── staging.yaml
    └── prod.yaml
```

### Chart.yaml

Declare the upstream Helm chart as a dependency:

```yaml
apiVersion: v2
name: my-component
description: My new component
version: 0.1.0
type: application
dependencies:
  - name: upstream-chart-name
    version: "1.2.3"
    repository: https://charts.example.com
```

### Values files

Create `values/base.yaml` with shared configuration:

```yaml
upstream-chart-name:
  someKey: someValue
```

!!! note
    Values must be nested under the dependency name (e.g., `upstream-chart-name:`). This is how Helm routes values to subchart dependencies.

Create environment-specific overrides in `values/dev.yaml`, `values/staging.yaml`, and `values/prod.yaml`:

```yaml
upstream-chart-name:
  replicas: 1
  resources:
    requests:
      cpu: 10m
      memory: 32Mi
```

## 2. Register in apps/values.yaml

Add an entry to the `components` map in `apps/values.yaml`:

```yaml
components:
  # ... existing components ...

  my-component:
    enabled: true
    namespace: my-namespace
    syncWave: "3"
```

Choose the sync wave based on dependencies:

| Wave | When to use |
|------|-------------|
| 0 | Core infrastructure (ArgoCD) |
| 1 | Secrets infrastructure (Vault, External Secrets) |
| 2 | CRDs, credentials, utilities |
| 3 | Application-level components |

### Optional properties

| Property | Default | Description |
|----------|---------|-------------|
| `hasValues` | `true` | Set to `false` if the component has no `values/base.yaml` or environment values |
| `createNamespace` | `true` | Set to `false` to skip `CreateNamespace=true` syncOption |
| `syncOptions` | `[]` | Extra syncOptions (e.g., `ServerSideApply=true`) |
| `ignoreDifferences` | `[]` | ArgoCD ignoreDifferences rules |

That's it — the dynamic template in `apps/templates/application.yaml` will automatically generate the ArgoCD `Application` resource for the new component. No template file needs to be created.

## 3. Verify with helm template

Before pushing, verify the Application template renders correctly:

```bash
helm template apps/ -f apps/values/dev.yaml
```

Check that:

- The Application CR is generated with correct metadata
- The sync wave is set appropriately
- The values file paths are correct
- The namespace matches your component's expectation

## 4. Push and sync

Commit and push. ArgoCD will detect the new Application in the App of Apps and deploy it.

```bash
git add components/my-component/ apps/values.yaml
git commit -m "feat: add my-component"
git push
```

## Disabling a component

To disable a component for a specific environment, override its `enabled` flag in the environment values file:

```yaml
# apps/values/dev.yaml
environment: dev
components:
  my-component:
    enabled: false
```

Helm deep-merges environment values with the defaults, so only the overridden property changes.
